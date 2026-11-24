package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVersion(t *testing.T) {
	if !versionGreater("1.1.0", "1.0.2") || versionGreater("1.0.2", "1.1.0") || versionGreater("1.1", "1.1.0") {
		t.Fatal("versionGreater")
	}
}

// Installe un faux client depuis un serveur local, puis vérifie qu'il est « à jour ».
func TestInstallAndCheck(t *testing.T) {
	files := map[string][]byte{"Azimov1.0.exe": []byte("exe"), "Data/frFR/a b.MPQ": make([]byte, 3<<20)}
	for i := range files["Data/frFR/a b.MPQ"] {
		files["Data/frFR/a b.MPQ"][i] = byte(i * 7)
	}
	m := Manifest{Name: "t", Version: "1", Exe: "Azimov1.0.exe", Realmlist: "127.0.0.1", Locale: "frFR", Base: "client/"}
	for p, b := range files {
		h := md5.Sum(b)
		m.Files = append(m.Files, FileEntry{P: p, S: int64(len(b)), H: hex.EncodeToString(h[:])})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/launcher/manifest.json", func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(m) })
	mux.HandleFunc("/launcher/client/", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path[len("/launcher/client/"):]
		b, ok := files[p]
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, p, time.Now(), bytesReader(b))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ManifestURL = srv.URL + "/launcher/manifest.json"
	dir := t.TempDir()
	os.Setenv("APPDATA", t.TempDir())
	e := NewEngine(nil)
	e.settings.GameDir = dir
	e.refresh(false)
	if s := e.Snapshot(); s.Phase != PhaseNeedInstall {
		t.Fatalf("phase %v %s", s.Phase, s.Status)
	}
	// reprise : un .azdl partiel déjà présent
	os.MkdirAll(filepath.Join(dir, "Data", "frFR"), 0o755)
	os.WriteFile(filepath.Join(dir, "Data", "frFR", "a b.MPQ.azdl"), files["Data/frFR/a b.MPQ"][:1<<20], 0o644)
	e.Download()
	if s := e.Snapshot(); s.Phase != PhaseReady {
		t.Fatalf("après téléchargement : %v %s", s.Phase, s.Status)
	}
	// fichier modifié -> détecté par la vérification complète
	os.WriteFile(filepath.Join(dir, "Azimov1.0.exe"), []byte("EXE"), 0o644)
	e.refresh(true)
	if s := e.Snapshot(); s.Phase != PhaseNeedUpdate {
		t.Fatalf("après modif : %v %s", s.Phase, s.Status)
	}
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
