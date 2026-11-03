package main

// Tray icon on Linux: StatusNotifierItem protocol and com.canonical.dbusmenu menu, on the D-Bus session bus.
// KDE Plasma (Steam Deck in Desktop mode) shows it; GNOME only with the AppIndicator extension.

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

var trayAvailable = true

const (
	sniPath  = dbus.ObjectPath("/StatusNotifierItem")
	menuPath = dbus.ObjectPath("/MenuBar")
)

// pixmap is an image for D-Bus: width, height, 32-bit ARGB pixels not premultiplied, bytes in network order.
type pixmap struct {
	L, H int32
	ARGB []byte
}

func iconPixmap(size int) []pixmap {
	img, _, err := image.Decode(bytes.NewReader(imageIco(iconICO, size)))
	if err != nil {
		return nil
	}
	b := img.Bounds()
	p := pixmap{int32(b.Dx()), int32(b.Dy()), make([]byte, 0, 4*b.Dx()*b.Dy())}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			p.ARGB = append(p.ARGB, c.A, c.R, c.G, c.B)
		}
	}
	return []pixmap{p}
}

// sniItem is the org.kde.StatusNotifierItem: left click = settings page; the menu goes through the Menu property.
type sniItem struct{ choose func(string) }

func (i sniItem) Activate(x, y int32) *dbus.Error          { go i.choose("reglages"); return nil }
func (i sniItem) SecondaryActivate(x, y int32) *dbus.Error { return nil }
func (i sniItem) ContextMenu(x, y int32) *dbus.Error       { return nil }
func (i sniItem) Scroll(delta int32, direction string) *dbus.Error {
	return nil
}

// menuEntry is a com.canonical.dbusmenu item: id, properties, children.
type menuEntry struct {
	ID       int32
	Props    map[string]dbus.Variant
	Children []dbus.Variant
}

type menuProperties struct {
	ID    int32
	Props map[string]dbus.Variant
}

// dbusMenu is the com.canonical.dbusmenu of the tray icon, flat; the id of an item is its position + 1, the root is 0.
type dbusMenu struct {
	lock     sync.Mutex
	read     func() []MenuItem
	choose   func(string)
	elements []MenuItem
	revision uint32
}

// refresh reads the state again; true if the menu changed (new revision).
func (m *dbusMenu) refresh() bool {
	l := m.read()
	m.lock.Lock()
	defer m.lock.Unlock()
	if slices.Equal(l, m.elements) {
		return false
	}
	m.elements = l
	m.revision++
	return true
}

func properties(e MenuItem) map[string]dbus.Variant {
	if e.Text == "" {
		return map[string]dbus.Variant{"type": dbus.MakeVariant("separator")}
	}
	return map[string]dbus.Variant{"label": dbus.MakeVariant(e.Text), "enabled": dbus.MakeVariant(e.Action != "")}
}

func (m *dbusMenu) entry(id int32) (menuEntry, bool) {
	if id == 0 {
		r := menuEntry{0, map[string]dbus.Variant{"children-display": dbus.MakeVariant("submenu")}, []dbus.Variant{}}
		for i, e := range m.elements {
			r.Children = append(r.Children, dbus.MakeVariant(menuEntry{int32(i + 1), properties(e), []dbus.Variant{}}))
		}
		return r, true
	}
	if id < 1 || int(id) > len(m.elements) {
		return menuEntry{}, false
	}
	return menuEntry{id, properties(m.elements[id-1]), []dbus.Variant{}}, true
}

func (m *dbusMenu) GetLayout(parent, depth int32, names []string) (uint32, menuEntry, *dbus.Error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	e, _ := m.entry(parent)
	return m.revision, e, nil
}

func (m *dbusMenu) GetGroupProperties(ids []int32, names []string) ([]menuProperties, *dbus.Error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	var l []menuProperties
	for _, id := range ids {
		if e, ok := m.entry(id); ok {
			l = append(l, menuProperties{id, e.Props})
		}
	}
	return l, nil
}

func (m *dbusMenu) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	if e, ok := m.entry(id); ok {
		if v, ok := e.Props[name]; ok {
			return v, nil
		}
	}
	return dbus.MakeVariant(""), nil
}

func (m *dbusMenu) Event(id int32, event string, dataDir dbus.Variant, playTime uint32) *dbus.Error {
	if event != "clicked" {
		return nil
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	if id >= 1 && int(id) <= len(m.elements) && m.elements[id-1].Action != "" {
		go m.choose(m.elements[id-1].Action)
	}
	return nil
}

func (m *dbusMenu) EventGroup(events []struct {
	ID    int32
	Event string
	Data  dbus.Variant
	Time  uint32
}) ([]int32, *dbus.Error) {
	for _, e := range events {
		m.Event(e.ID, e.Event, e.Data, e.Time)
	}
	return []int32{}, nil
}

// AboutToShow: the menu is about to open; true if it must be read again (pending Uploads, local Server).
func (m *dbusMenu) AboutToShow(id int32) (bool, *dbus.Error) { return m.refresh(), nil }

func (m *dbusMenu) AboutToShowGroup(ids []int32) ([]int32, []int32, *dbus.Error) {
	if m.refresh() {
		return []int32{0}, []int32{}, nil
	}
	return []int32{}, []int32{}, nil
}

// showIcon registers the tray icon with the desktop's StatusNotifierWatcher, then serves its calls until end.
func showIcon(menu func() []MenuItem, choose func(string), end <-chan struct{}) error {
	fail := func(err error) error {
		return fmt.Errorf("%s\n%s%v", tr("Impossible d'afficher l'icône près de l'horloge : votre bureau n'a pas de zone de notification compatible. "+
			"Sous GNOME, installez l'extension « AppIndicator and KStatusNotifierItem Support ». Pour ne plus voir ce message, décochez « Afficher l'icône près de l'horloge » dans les réglages.",
			"Can't show the icon near the clock: your desktop has no compatible notification area. "+
				"On GNOME, install the \"AppIndicator and KStatusNotifierItem Support\" extension. To stop seeing this message, untick \"Show the icon near the clock\" in the settings."),
			tr("Détail : ", "Detail: "), err)
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fail(err)
	}
	defer conn.Close()
	item := sniItem{choose}
	m := &dbusMenu{read: menu, choose: choose}
	m.refresh()
	tooltip := struct {
		Icon   string
		Images []pixmap
		Title  string
		Detail string
	}{"", []pixmap{}, "3dssavesync", ""}
	props, err := prop.Export(conn, sniPath, prop.Map{"org.kde.StatusNotifierItem": {
		"Category": {Value: "ApplicationStatus"}, "Id": {Value: "3dssavesync"}, "Title": {Value: "3dssavesync"},
		"Status": {Value: "Active"}, "IconName": {Value: ""}, "IconPixmap": {Value: iconPixmap(64)}, "IconThemePath": {Value: ""},
		"ItemIsMenu": {Value: false}, "Menu": {Value: menuPath}, "ToolTip": {Value: tooltip},
	}})
	if err != nil {
		return fail(err)
	}
	propsMenu, err := prop.Export(conn, menuPath, prop.Map{"com.canonical.dbusmenu": {
		"Version": {Value: uint32(3)}, "TextDirection": {Value: "ltr"}, "Status": {Value: "normal"}, "IconThemePath": {Value: []string{}},
	}})
	if err != nil {
		return fail(err)
	}
	for _, x := range []struct {
		object any
		path   dbus.ObjectPath
		iface  string
		props  *prop.Properties
	}{{item, sniPath, "org.kde.StatusNotifierItem", props}, {m, menuPath, "com.canonical.dbusmenu", propsMenu}} {
		node := &introspect.Node{Name: string(x.path), Interfaces: []introspect.Interface{introspect.IntrospectData, prop.IntrospectData,
			{Name: x.iface, Methods: introspect.Methods(x.object), Properties: x.props.Introspection(x.iface)}}}
		if err := conn.Export(x.object, x.path, x.iface); err != nil {
			return fail(err)
		}
		if err := conn.Export(introspect.NewIntrospectable(node), x.path, "org.freedesktop.DBus.Introspectable"); err != nil {
			return fail(err)
		}
	}
	name := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
	if _, err := conn.RequestName(name, dbus.NameFlagDoNotQueue); err != nil {
		return fail(err)
	}
	register := func() error {
		return conn.Object("org.kde.StatusNotifierWatcher", "/StatusNotifierWatcher").Call("org.kde.StatusNotifierWatcher.RegisterStatusNotifierItem", 0, name).Err
	}
	if err := register(); err != nil {
		return fail(err)
	}
	// Desktop restarted (Plasma, Steam Deck switch to Desktop mode): register again with the new Watcher.
	conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, "org.kde.StatusNotifierWatcher"))
	signals := make(chan *dbus.Signal, 10)
	conn.Signal(signals)
	timer := time.NewTicker(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-end:
			return nil
		case s, ok := <-signals:
			if !ok {
				return fail(errors.New("D-Bus : connexion perdue"))
			}
			if len(s.Body) == 3 && s.Body[2] != "" {
				register()
			}
		case <-timer.C:
			if m.refresh() {
				m.lock.Lock()
				r := m.revision
				m.lock.Unlock()
				conn.Emit(menuPath, "com.canonical.dbusmenu.LayoutUpdated", r, int32(0))
			}
		}
	}
}
