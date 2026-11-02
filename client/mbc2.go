package main

// MBC2 (Game Boy) : 512 valeurs de 4 bits (docs/research-eeprom-mbc2.md, ADR 0012). mGBA écrit 256 octets, deux
// valeurs par octet (adresse paire dans le quartet bas) ; SameBoy, VBA-M et BGB 512 octets, une valeur par octet.
// Forme du serveur : 512 octets, quartet haut à 0. Sur PC, un fichier GB de 256 ou 512 octets ne peut être que MBC2.

func estMBC2(systeme string, n int) bool { return systeme == "GB" && (n == 256 || n == 512) }

// normaliserMBC2 : fichier de 256 ou 512 octets -> forme du serveur.
func normaliserMBC2(f []byte) []byte {
	s := make([]byte, 512)
	for i := range s {
		if len(f) == 256 {
			s[i] = f[i/2] >> (i % 2 * 4) & 0x0F
		} else {
			s[i] = f[i] & 0x0F
		}
	}
	return s
}

// fichierMBC2 : forme du serveur -> fichier de n octets : 256 (mGBA) ou 512 (quartet haut à 0xF, lu par SameBoy,
// VBA-M et BGB).
func fichierMBC2(s []byte, n int) []byte {
	f := make([]byte, n)
	for i, v := range s {
		if n == 256 {
			f[i/2] |= v & 0x0F << (i % 2 * 4)
		} else {
			f[i] = v&0x0F | 0xF0
		}
	}
	return f
}

// Taille d'un fichier MBC2 neuf : 256 octets chez mGBA (application ou cœur RetroArch), 512 ailleurs.
func (s *Session) tailleMBC2(j *Jeu) int {
	p := s.profil
	if e := s.cfg.Emulateurs[p]; e != nil && e.Profil != "" {
		p = e.Profil
	}
	if p == "mgba" || j.Coeur == "mGBA" {
		return 256
	}
	return 512
}
