package main

// Icône près de l'horloge sous Linux : protocole StatusNotifierItem et menu com.canonical.dbusmenu, sur le bus D-Bus de
// session. KDE Plasma (Steam Deck en mode Bureau) l'affiche ; GNOME seulement avec l'extension AppIndicator.

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

var trayDisponible = true

const (
	cheminSNI  = dbus.ObjectPath("/StatusNotifierItem")
	cheminMenu = dbus.ObjectPath("/MenuBar")
)

// Image pour D-Bus : largeur, hauteur, pixels ARGB 32 bits non prémultipliés, octets dans l'ordre du réseau.
type pixmap struct {
	L, H int32
	ARGB []byte
}

func pixmapIcone(taille int) []pixmap {
	img, _, err := image.Decode(bytes.NewReader(imageIco(iconeIco, taille)))
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

// org.kde.StatusNotifierItem : clic gauche = réglages ; le menu passe par la propriété Menu.
type itemSNI struct{ choisir func(string) }

func (i itemSNI) Activate(x, y int32) *dbus.Error          { go i.choisir("reglages"); return nil }
func (i itemSNI) SecondaryActivate(x, y int32) *dbus.Error { return nil }
func (i itemSNI) ContextMenu(x, y int32) *dbus.Error       { return nil }
func (i itemSNI) Scroll(delta int32, sens string) *dbus.Error {
	return nil
}

// Élément de com.canonical.dbusmenu : id, propriétés, enfants.
type entreeMenu struct {
	ID      int32
	Props   map[string]dbus.Variant
	Enfants []dbus.Variant
}

type proprietesMenu struct {
	ID    int32
	Props map[string]dbus.Variant
}

// com.canonical.dbusmenu : le menu de l'icône, à plat ; l'id d'un élément est sa position + 1, la racine est 0.
type menuDBus struct {
	verrou   sync.Mutex
	lire     func() []ElementMenu
	choisir  func(string)
	elements []ElementMenu
	revision uint32
}

// rafraichir : relit l'état ; vrai si le menu a changé (nouvelle révision).
func (m *menuDBus) rafraichir() bool {
	l := m.lire()
	m.verrou.Lock()
	defer m.verrou.Unlock()
	if slices.Equal(l, m.elements) {
		return false
	}
	m.elements = l
	m.revision++
	return true
}

func proprietes(e ElementMenu) map[string]dbus.Variant {
	if e.Texte == "" {
		return map[string]dbus.Variant{"type": dbus.MakeVariant("separator")}
	}
	return map[string]dbus.Variant{"label": dbus.MakeVariant(e.Texte), "enabled": dbus.MakeVariant(e.Action != "")}
}

func (m *menuDBus) entree(id int32) (entreeMenu, bool) {
	if id == 0 {
		r := entreeMenu{0, map[string]dbus.Variant{"children-display": dbus.MakeVariant("submenu")}, []dbus.Variant{}}
		for i, e := range m.elements {
			r.Enfants = append(r.Enfants, dbus.MakeVariant(entreeMenu{int32(i + 1), proprietes(e), []dbus.Variant{}}))
		}
		return r, true
	}
	if id < 1 || int(id) > len(m.elements) {
		return entreeMenu{}, false
	}
	return entreeMenu{id, proprietes(m.elements[id-1]), []dbus.Variant{}}, true
}

func (m *menuDBus) GetLayout(parent, profondeur int32, noms []string) (uint32, entreeMenu, *dbus.Error) {
	m.verrou.Lock()
	defer m.verrou.Unlock()
	e, _ := m.entree(parent)
	return m.revision, e, nil
}

func (m *menuDBus) GetGroupProperties(ids []int32, noms []string) ([]proprietesMenu, *dbus.Error) {
	m.verrou.Lock()
	defer m.verrou.Unlock()
	var l []proprietesMenu
	for _, id := range ids {
		if e, ok := m.entree(id); ok {
			l = append(l, proprietesMenu{id, e.Props})
		}
	}
	return l, nil
}

func (m *menuDBus) GetProperty(id int32, nom string) (dbus.Variant, *dbus.Error) {
	m.verrou.Lock()
	defer m.verrou.Unlock()
	if e, ok := m.entree(id); ok {
		if v, ok := e.Props[nom]; ok {
			return v, nil
		}
	}
	return dbus.MakeVariant(""), nil
}

func (m *menuDBus) Event(id int32, evenement string, donnees dbus.Variant, temps uint32) *dbus.Error {
	if evenement != "clicked" {
		return nil
	}
	m.verrou.Lock()
	defer m.verrou.Unlock()
	if id >= 1 && int(id) <= len(m.elements) && m.elements[id-1].Action != "" {
		go m.choisir(m.elements[id-1].Action)
	}
	return nil
}

func (m *menuDBus) EventGroup(evenements []struct {
	ID        int32
	Evenement string
	Donnees   dbus.Variant
	Temps     uint32
}) ([]int32, *dbus.Error) {
	for _, e := range evenements {
		m.Event(e.ID, e.Evenement, e.Donnees, e.Temps)
	}
	return []int32{}, nil
}

// AboutToShow : le menu va s'ouvrir ; vrai s'il faut le relire (Envois en attente, serveur local).
func (m *menuDBus) AboutToShow(id int32) (bool, *dbus.Error) { return m.rafraichir(), nil }

func (m *menuDBus) AboutToShowGroup(ids []int32) ([]int32, []int32, *dbus.Error) {
	if m.rafraichir() {
		return []int32{0}, []int32{}, nil
	}
	return []int32{}, []int32{}, nil
}

// afficherIcone : inscrit l'icône auprès du StatusNotifierWatcher du bureau, puis sert ses appels jusqu'à la fin.
func afficherIcone(menu func() []ElementMenu, choisir func(string), fin <-chan struct{}) error {
	echec := func(err error) error {
		return fmt.Errorf("%s\n%s%v", tr("Impossible d'afficher l'icône près de l'horloge : votre bureau n'a pas de zone de notification compatible. "+
			"Sous GNOME, installez l'extension « AppIndicator and KStatusNotifierItem Support ». Pour ne plus voir ce message, décochez « Afficher l'icône près de l'horloge » dans les réglages.",
			"Can't show the icon near the clock: your desktop has no compatible notification area. "+
				"On GNOME, install the \"AppIndicator and KStatusNotifierItem Support\" extension. To stop seeing this message, untick \"Show the icon near the clock\" in the settings."),
			tr("Détail : ", "Detail: "), err)
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return echec(err)
	}
	defer conn.Close()
	item := itemSNI{choisir}
	m := &menuDBus{lire: menu, choisir: choisir}
	m.rafraichir()
	infobulle := struct {
		Icone  string
		Images []pixmap
		Titre  string
		Detail string
	}{"", []pixmap{}, "3dssavesync", ""}
	props, err := prop.Export(conn, cheminSNI, prop.Map{"org.kde.StatusNotifierItem": {
		"Category": {Value: "ApplicationStatus"}, "Id": {Value: "3dssavesync"}, "Title": {Value: "3dssavesync"},
		"Status": {Value: "Active"}, "IconName": {Value: ""}, "IconPixmap": {Value: pixmapIcone(64)}, "IconThemePath": {Value: ""},
		"ItemIsMenu": {Value: false}, "Menu": {Value: cheminMenu}, "ToolTip": {Value: infobulle},
	}})
	if err != nil {
		return echec(err)
	}
	propsMenu, err := prop.Export(conn, cheminMenu, prop.Map{"com.canonical.dbusmenu": {
		"Version": {Value: uint32(3)}, "TextDirection": {Value: "ltr"}, "Status": {Value: "normal"}, "IconThemePath": {Value: []string{}},
	}})
	if err != nil {
		return echec(err)
	}
	for _, x := range []struct {
		objet  any
		chemin dbus.ObjectPath
		iface  string
		props  *prop.Properties
	}{{item, cheminSNI, "org.kde.StatusNotifierItem", props}, {m, cheminMenu, "com.canonical.dbusmenu", propsMenu}} {
		noeud := &introspect.Node{Name: string(x.chemin), Interfaces: []introspect.Interface{introspect.IntrospectData, prop.IntrospectData,
			{Name: x.iface, Methods: introspect.Methods(x.objet), Properties: x.props.Introspection(x.iface)}}}
		if err := conn.Export(x.objet, x.chemin, x.iface); err != nil {
			return echec(err)
		}
		if err := conn.Export(introspect.NewIntrospectable(noeud), x.chemin, "org.freedesktop.DBus.Introspectable"); err != nil {
			return echec(err)
		}
	}
	nom := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
	if _, err := conn.RequestName(nom, dbus.NameFlagDoNotQueue); err != nil {
		return echec(err)
	}
	inscrire := func() error {
		return conn.Object("org.kde.StatusNotifierWatcher", "/StatusNotifierWatcher").Call("org.kde.StatusNotifierWatcher.RegisterStatusNotifierItem", 0, nom).Err
	}
	if err := inscrire(); err != nil {
		return echec(err)
	}
	// Bureau redémarré (Plasma, passage au mode Bureau du Steam Deck) : nouvelle inscription auprès du nouveau Watcher.
	conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, "org.kde.StatusNotifierWatcher"))
	signaux := make(chan *dbus.Signal, 10)
	conn.Signal(signaux)
	minuterie := time.NewTicker(2 * time.Second)
	defer minuterie.Stop()
	for {
		select {
		case <-fin:
			return nil
		case s, ok := <-signaux:
			if !ok {
				return echec(errors.New("D-Bus : connexion perdue"))
			}
			if len(s.Body) == 3 && s.Body[2] != "" {
				inscrire()
			}
		case <-minuterie.C:
			if m.rafraichir() {
				m.verrou.Lock()
				r := m.revision
				m.verrou.Unlock()
				conn.Emit(cheminMenu, "com.canonical.dbusmenu.LayoutUpdated", r, int32(0))
			}
		}
	}
}
