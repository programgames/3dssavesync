package main

// ES-DE (custom_systems/es_systems.xml and es_find_rules.xml): install (esde) and uninstall (undoEsde) of the
// 3DSSAVESYNC rule and the "3dssavesync" commands. Sources: docs/notes/research-v3-frontends.md, ADR 0019.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	esdeRule    = regexp.MustCompile(`(?s)  <emulator name="3DSSAVESYNC">.*?</emulator>\r?\n`)
	esdeCommand = regexp.MustCompile(`<command label="3dssavesync \([^"]*\)">[^<]*</command>\r?\n[ \t]*`)
	esdeSystem  = regexp.MustCompile(`(?s)[ \t]*<system>.*?</system>\r?\n?`)
	// esdeBlocks: the <system> block of each ES-DE system of esdeSystems.
	esdeBlocks = func() map[string]*regexp.Regexp {
		m := map[string]*regexp.Regexp{}
		for n := range esdeSystems {
			m[n] = regexp.MustCompile(`(?s)<system>\s*<name>` + n + `</name>.*?</system>`)
		}
		return m
	}()
)

// esdeSystems maps ES-DE systems -> System, full name, extensions read by the Launcher.
var esdeSystems = map[string][3]string{
	"nds":  {"DS", "Nintendo DS", ".nds .NDS .zip .ZIP"},
	"gb":   {"GB", "Nintendo Game Boy", ".gb .GB .zip .ZIP"},
	"gbc":  {"GB", "Nintendo Game Boy Color", ".gbc .GBC .zip .ZIP"},
	"gba":  {"GBA", "Nintendo Game Boy Advance", ".gba .GBA .zip .ZIP"},
	"n3ds": {"3DS", "Nintendo 3DS", ".3ds .3DS .cci .CCI .cxi .CXI"},
}

// esde writes the 3DSSAVESYNC rule in es_find_rules.xml, and a "3dssavesync" command at the top of each handled system
// in es_systems.xml (first command = default emulator). A system missing from the custom file gets a minimal block
// (the block shipped with ES-DE can't be read inside an AppImage).
// emus: System -> name of the config.json entry.
func esde(systems, rules, launcher string, emus map[System]string) (string, string, []string) {
	var report []string
	rule := "  <emulator name=\"3DSSAVESYNC\">\n    <rule type=\"staticpath\">\n      <entry>" + escapeXML(launcher) + "</entry>\n    </rule>\n  </emulator>\n"
	switch {
	case esdeRule.MatchString(rules):
		rules = esdeRule.ReplaceAllLiteralString(rules, rule)
	case strings.Contains(rules, "</ruleList>"):
		rules = strings.Replace(rules, "</ruleList>", rule+"</ruleList>", 1)
	default:
		rules = "<?xml version=\"1.0\"?>\n<ruleList>\n" + rule + "</ruleList>\n"
	}
	if !strings.Contains(systems, "</systemList>") {
		systems = "<?xml version=\"1.0\"?>\n<systemList>\n</systemList>\n"
	}
	var names []string
	for n := range esdeSystems {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		d := esdeSystems[n]
		name := emus[System(d[0])]
		if name == "" {
			continue
		}
		command := `<command label="3dssavesync (` + name + `)">%EMULATOR_3DSSAVESYNC% run --emulator ` + name + ` %ROM%</command>`
		block := esdeBlocks[n].FindString(systems)
		switch {
		case strings.Contains(block, "--emulator "+name+" "):
			report = append(report, fmt.Sprintf(tr("ES-DE %s : déjà en place", "ES-DE %s: already set up"), n))
		case block != "" && strings.Contains(block, "<command"):
			i := strings.Index(block, "<command")
			systems = strings.Replace(systems, block, block[:i]+command+"\n        "+block[i:], 1)
			report = append(report, fmt.Sprintf(tr("ES-DE %s : commande ajoutée en tête de liste", "ES-DE %s: command added at the top of the list"), n))
		default:
			ext := d[2]
			if block != "" {
				if e := field(block, "extension"); e != "" { // the user's list kept, never shortened
					ext = e
				}
				systems = strings.Replace(systems, block, "", 1)
			}
			newText := "    <system>\n        <name>" + n + "</name>\n        <fullname>" + d[1] + "</fullname>\n        <path>%ROMPATH%/" + n +
				"</path>\n        <extension>" + ext + "</extension>\n        " + command + "\n        <platform>" + n +
				"</platform>\n        <theme>" + n + "</theme>\n    </system>\n"
			systems = strings.Replace(systems, "</systemList>", newText+"</systemList>", 1)
			report = append(report, fmt.Sprintf(tr("ES-DE %s : système ajouté, avec seulement la commande 3dssavesync", "ES-DE %s: system added, with the 3dssavesync command only"), n))
		}
	}
	return systems, rules, report
}

// undoEsde removes the 3DSSAVESYNC rule and the 3dssavesync commands. A system with no command left is a minimal
// block that esde added: it is removed, and ES-DE goes back to its own setting for this system.
func undoEsde(systems, rules string) (string, string, []string) {
	var report []string
	if esdeRule.MatchString(rules) {
		rules = esdeRule.ReplaceAllLiteralString(rules, "")
	}
	systems = esdeSystem.ReplaceAllStringFunc(systems, func(b string) string {
		if !esdeCommand.MatchString(b) {
			return b
		}
		n := field(b, "name")
		b = esdeCommand.ReplaceAllLiteralString(b, "")
		if !strings.Contains(b, "<command") {
			report = append(report, fmt.Sprintf(tr("%s : système ajouté par 3dssavesync retiré", "%s: removed the system added by 3dssavesync"), n))
			return ""
		}
		report = append(report, fmt.Sprintf(tr("%s : commande 3dssavesync retirée", "%s: removed the 3dssavesync command"), n))
		return b
	})
	return systems, rules, report
}
