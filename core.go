// Cœur du launcher : configuration, manifeste, téléchargement, vérification, lancement du jeu.
package main

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Version du launcher : doit suivre $LAUNCHER_VERSION de generer_manifest.php.
const AppVersion = "1.1.0"

// Adresses par défaut (modifiables avec launcher.ini à côté de l'exe).
var (
	ManifestURL = "http://163.172.102.172/launcher/manifest.json"
	APIURL      = "http://163.172.102.172/api.php"
	SiteURL     = "http://163.172.102.172/"
	VoteURL     = "" // vide = SiteURL + "voter.php" (ou le champ « vote » de api.php)
	RegisterURL = "" // vide = SiteURL + "inscription.php" (ou le champ « register » de api.php)
)

const (
	downloadWorkers = 3
	maxRetries      = 5
	stallTimeout    = 60 * time.Second
	stateDir        = ".azimov"
)

// ---------------------------------------------------------------- données

type FileEntry struct {
	P string `json:"p"` // chemin relatif (avec /)
	S int64  `json:"s"` // taille
	H string `json:"h"` // MD5
}

type LauncherInfo struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	H       string `json:"h"`
	S       int64  `json:"s"`
}

type Manifest struct {
	Name      string        `json:"name"`
	Version   string        `json:"version"`
	Exe       string        `json:"exe"`
	Realmlist string        `json:"realmlist"`
	Locale    string        `json:"locale"`
	Base      string        `json:"base"`
	Total     int64         `json:"total"`
	Delete    []string      `json:"delete"`
	Launcher  *LauncherInfo `json:"launcher"`
	Files     []FileEntry   `json:"files"`
}

type NewsItem struct {
	Date  string `json:"date"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

type ServerInfo struct {
	Server    string     `json:"server"`
	Realm     string     `json:"realm"`
	Auth      bool       `json:"auth"`
	World     bool       `json:"world"`
	Online    *int       `json:"online"`
	Realmlist string     `json:"realmlist"`
	Site      string     `json:"site"`
	Register  string     `json:"register"`
	Vote      string     `json:"vote"`
	News      []NewsItem `json:"news"`
}

type Settings struct {
	GameDir    string `json:"dir"`
	ClearCache bool   `json:"clearCache"`
}

type stateEntry struct {
	S int64  `json:"s"`
	M int64  `json:"m"`
	H string `json:"h"`
}

// ---------------------------------------------------------------- utilitaires

func itoa(v int) string     { return strconv.Itoa(v) }
func ftoa(v float32) string { return strconv.FormatFloat(float64(v), 'f', 1, 32) }

func humanSize(b int64) string {
	f := float64(b)
	switch {
	case f >= 1<<30:
		return strings.Replace(fmt.Sprintf("%.2f Go", f/(1<<30)), ".", ",", 1)
	case f >= 1<<20:
		return strings.Replace(fmt.Sprintf("%.1f Mo", f/(1<<20)), ".", ",", 1)
	default:
		return fmt.Sprintf("%.0f Ko", f/(1<<10))
	}
}

func humanDuration(sec float64) string {
	if sec < 0 || sec > 360000 {
		return ""
	}
	s := int(sec)
	if s >= 3600 {
		return fmt.Sprintf("%d h %02d min", s/3600, (s%3600)/60)
	}
	if s >= 60 {
		return fmt.Sprintf("%d min %02d s", s/60, s%60)
	}
	return fmt.Sprintf("%d s", s)
}

// Compare deux versions « 1.2.3 ».
func versionGreater(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func md5File(p string, progress func(int64)) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	buf := make([]byte, 1<<20)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
			if progress != nil {
				progress(int64(n))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// launcher.ini (facultatif, à côté de l'exe) : manifest=, api=, site=, vote=, register=
func loadIni() {
	f, err := os.Open(filepath.Join(exeDir(), "launcher.ini"))
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		switch k {
		case "manifest":
			ManifestURL = v
		case "api":
			APIURL = v
		case "site":
			SiteURL = v
		case "vote":
			VoteURL = v
		case "register":
			RegisterURL = v
		}
	}
}

func settingsPath() string {
	base := os.Getenv("APPDATA")
	if base == "" {
		base = exeDir()
	}
	return filepath.Join(base, "AzimovLauncher", "settings.json")
}

func loadSettings() Settings {
	s := Settings{ClearCache: true}
	if b, err := os.ReadFile(settingsPath()); err == nil {
		json.Unmarshal(b, &s)
	}
	if s.GameDir == "" {
		// l'exe est posé dans un client existant ? on l'utilise
		if _, err := os.Stat(filepath.Join(exeDir(), "Data")); err == nil {
			s.GameDir = exeDir()
		} else {
			s.GameDir = filepath.Join(os.Getenv("SystemDrive")+`\`, "Jeux", "Azimov Online")
		}
	}
	return s
}

func saveSettings(s Settings) {
	p := settingsPath()
	os.MkdirAll(filepath.Dir(p), 0o755)
	b, _ := json.MarshalIndent(s, "", "  ")
	os.WriteFile(p, b, 0o644)
}

var httpClient = &http.Client{Transport: &http.Transport{
	Proxy:               http.ProxyFromEnvironment,
	MaxIdleConnsPerHost: downloadWorkers + 2,
	DisableCompression:  true,
	IdleConnTimeout:     90 * time.Second,
}}

func getJSON(u string, v any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "AzimovLauncher/"+AppVersion)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("erreur HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// Adresse d'un fichier du client : dossier du manifeste + base + chemin encodé.
func fileURL(m *Manifest, rel string) string {
	u, err := url.Parse(ManifestURL)
	if err != nil {
		return ""
	}
	// chemin en clair : url.URL se charge de l'encodage (espaces, accents, #…)
	u.Path = path.Join(path.Dir(u.Path), m.Base, rel)
	u.RawPath, u.RawQuery = "", ""
	return u.String()
}

// ---------------------------------------------------------------- moteur

type Phase int

const (
	PhaseConnecting  Phase = iota // lecture du manifeste
	PhaseChecking                 // contrôle des fichiers
	PhaseNeedInstall              // jeu absent
	PhaseNeedUpdate               // fichiers à télécharger
	PhaseDownloading
	PhaseReady
	PhaseLaunching
	PhaseSelfUpdate
	PhaseError
)

type Snapshot struct {
	Phase      Phase
	Status     string
	Detail     string
	Progress   float64 // 0..1, -1 = indéterminé
	Busy       bool
	Server     *ServerInfo
	Manifest   *Manifest
	Settings   Settings
	ToDownload int64
}

type Engine struct {
	mu       sync.Mutex
	snap     Snapshot
	manifest *Manifest
	settings Settings
	server   *ServerInfo
	cancel   context.CancelFunc
	missing  []FileEntry
	notify   func() // demande de rafraîchissement de la fenêtre
}

func NewEngine(notify func()) *Engine {
	e := &Engine{notify: notify, settings: loadSettings()}
	e.snap.Settings = e.settings
	e.snap.Progress = -1
	return e
}

func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.snap
	s.Server = e.server
	s.Manifest = e.manifest
	s.Settings = e.settings
	return s
}

func (e *Engine) set(fn func(s *Snapshot)) {
	e.mu.Lock()
	fn(&e.snap)
	e.mu.Unlock()
	if e.notify != nil {
		e.notify()
	}
}

func (e *Engine) fail(msg string) {
	e.set(func(s *Snapshot) {
		s.Phase, s.Status, s.Detail, s.Busy, s.Progress = PhaseError, msg, "", false, -1
	})
}

func (e *Engine) VoteURL() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if VoteURL != "" {
		return VoteURL
	}
	if e.server != nil && e.server.Vote != "" {
		return e.server.Vote
	}
	return strings.TrimRight(SiteURL, "/") + "/voter.php"
}

func (e *Engine) RegisterURL() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if RegisterURL != "" {
		return RegisterURL
	}
	if e.server != nil && e.server.Register != "" {
		return e.server.Register
	}
	return strings.TrimRight(SiteURL, "/") + "/inscription.php"
}

func (e *Engine) SiteURL() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.server != nil && e.server.Site != "" {
		return e.server.Site
	}
	return SiteURL
}

// Statut du royaume + actualités (toutes les 60 s).
func (e *Engine) StartServerInfo() {
	go func() {
		for {
			var si ServerInfo
			if err := getJSON(APIURL, &si); err == nil {
				e.mu.Lock()
				e.server = &si
				e.mu.Unlock()
			} else {
				e.mu.Lock()
				if e.server == nil {
					e.server = &ServerInfo{Realm: "Azimov"}
				} else {
					e.server.Auth, e.server.World = false, false
				}
				e.mu.Unlock()
			}
			if e.notify != nil {
				e.notify()
			}
			time.Sleep(60 * time.Second)
		}
	}()
}

// Démarrage : manifeste, mise à jour du launcher, contrôle rapide.
func (e *Engine) Start() {
	os.Remove(selfPath() + ".old")
	go e.refresh(false)
}

func (e *Engine) refresh(fullCheck bool) {
	e.set(func(s *Snapshot) {
		s.Phase, s.Status, s.Detail, s.Busy, s.Progress = PhaseConnecting, "Connexion au serveur de mise à jour…", "", true, -1
	})
	var m Manifest
	if err := getJSON(ManifestURL, &m); err != nil {
		e.fail("Serveur de mise à jour injoignable. Réessaie plus tard.")
		return
	}
	e.mu.Lock()
	e.manifest = &m
	e.mu.Unlock()

	if m.Launcher != nil && m.Launcher.URL != "" && versionGreater(m.Launcher.Version, AppVersion) {
		if h, _ := md5File(selfPath(), nil); !strings.EqualFold(h, m.Launcher.H) {
			if e.selfUpdate(&m) {
				return
			}
		}
	}
	e.check(&m, fullCheck)
}

func (e *Engine) gameDir() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.settings.GameDir
}

func statePath(dir string) string { return filepath.Join(dir, stateDir, "etat.json") }

func loadState(dir string) map[string]stateEntry {
	st := map[string]stateEntry{}
	if b, err := os.ReadFile(statePath(dir)); err == nil {
		json.Unmarshal(b, &st)
	}
	return st
}

func saveState(dir string, st map[string]stateEntry) {
	os.MkdirAll(filepath.Join(dir, stateDir), 0o755)
	b, _ := json.Marshal(st)
	tmp := statePath(dir) + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		os.Remove(statePath(dir))
		os.Rename(tmp, statePath(dir))
	}
}

// Contrôle des fichiers. fullCheck = recalcule toutes les empreintes.
func (e *Engine) check(m *Manifest, fullCheck bool) {
	dir := e.gameDir()
	st := loadState(dir)
	if fullCheck {
		st = map[string]stateEntry{}
	}

	// fichiers à supprimer chez le joueur
	for _, d := range m.Delete {
		if d != "" && !strings.Contains(d, "..") {
			os.Remove(filepath.Join(dir, filepath.FromSlash(d)))
		}
	}

	var toHash []FileEntry
	var hashBytes int64
	var missing []FileEntry
	var logLines []string
	present := 0
	for _, f := range m.Files {
		fp := filepath.Join(dir, filepath.FromSlash(f.P))
		fi, err := os.Stat(fp)
		if err != nil {
			missing = append(missing, f)
			logLines = append(logLines, "absent : "+f.P)
			continue
		}
		present++
		if fi.Size() != f.S {
			missing = append(missing, f)
			logLines = append(logLines, "taille différente : "+f.P)
			continue
		}
		if c, ok := st[f.P]; ok && c.S == f.S && c.M == fi.ModTime().Unix() && strings.EqualFold(c.H, f.H) {
			continue
		}
		toHash = append(toHash, f)
		hashBytes += f.S
	}

	if len(toHash) > 0 {
		var done int64
		start := time.Now()
		lastUI := start
		e.set(func(s *Snapshot) {
			s.Phase, s.Status, s.Busy, s.Progress = PhaseChecking, "Vérification des fichiers…", true, 0
		})
		for _, f := range toHash {
			fp := filepath.Join(dir, filepath.FromSlash(f.P))
			name := f.P
			h, err := md5File(fp, func(n int64) {
				d := atomic.AddInt64(&done, n)
				if time.Since(lastUI) > 200*time.Millisecond {
					lastUI = time.Now()
					el := time.Since(start).Seconds()
					e.set(func(s *Snapshot) {
						s.Progress = float64(d) / float64(hashBytes)
						s.Detail = fmt.Sprintf("%s — %s / %s", name, humanSize(d), humanSize(hashBytes))
						if el > 2 && d > 0 {
							s.Detail += " — reste " + humanDuration(el/float64(d)*float64(hashBytes-d))
						}
					})
				}
			})
			if err != nil || !strings.EqualFold(h, f.H) {
				missing = append(missing, f)
				logLines = append(logLines, "contenu différent : "+f.P)
				delete(st, f.P)
				continue
			}
			if fi, err := os.Stat(fp); err == nil {
				st[f.P] = stateEntry{S: f.S, M: fi.ModTime().Unix(), H: f.H}
			}
		}
		saveState(dir, st)
	}
	writeCheckLog(dir, logLines)

	var need int64
	for _, f := range missing {
		need += f.S
	}
	e.mu.Lock()
	e.missing = missing
	e.mu.Unlock()

	switch {
	case len(missing) == 0:
		e.set(func(s *Snapshot) {
			s.Phase, s.Status, s.Detail, s.Busy, s.Progress, s.ToDownload = PhaseReady, "Le jeu est à jour. Bon jeu !", "", false, 1, 0
		})
	case present == 0:
		e.set(func(s *Snapshot) {
			s.Phase, s.Status, s.Busy, s.Progress, s.ToDownload = PhaseNeedInstall, "Le jeu n'est pas encore installé dans ce dossier.", false, 0, need
			s.Detail = "Téléchargement complet : " + humanSize(need)
		})
	default:
		e.set(func(s *Snapshot) {
			s.Phase, s.Status, s.Busy, s.Progress, s.ToDownload = PhaseNeedUpdate, "Une mise à jour est disponible.", false, 0, need
			s.Detail = fmt.Sprintf("%d fichier(s) à télécharger : %s", len(missing), humanSize(need))
		})
	}
}

func writeCheckLog(dir string, lines []string) {
	if len(lines) == 0 {
		return
	}
	os.MkdirAll(filepath.Join(dir, stateDir), 0o755)
	head := time.Now().Format("02/01/2006 15:04:05") + "\n"
	os.WriteFile(filepath.Join(dir, stateDir, "verification.log"), []byte(head+strings.Join(lines, "\n")+"\n"), 0o644)
}

// Vérification complète (bouton « Vérifier les fichiers »).
func (e *Engine) Verify() {
	if e.Snapshot().Busy {
		return
	}
	go e.refresh(true)
}

// Change de dossier et relance le contrôle.
func (e *Engine) SetGameDir(dir string) {
	e.mu.Lock()
	e.settings.GameDir = dir
	s := e.settings
	e.mu.Unlock()
	saveSettings(s)
	go e.refresh(false)
}

func (e *Engine) SetClearCache(v bool) {
	e.mu.Lock()
	e.settings.ClearCache = v
	s := e.settings
	e.mu.Unlock()
	saveSettings(s)
	if e.notify != nil {
		e.notify()
	}
}

func (e *Engine) Cancel() {
	e.mu.Lock()
	c := e.cancel
	e.mu.Unlock()
	if c != nil {
		c()
	}
}

// Téléchargement des fichiers manquants.
func (e *Engine) Download() {
	e.mu.Lock()
	m := e.manifest
	files := append([]FileEntry(nil), e.missing...)
	dir := e.settings.GameDir
	e.mu.Unlock()
	if m == nil || len(files) == 0 {
		return
	}
	var total int64
	for _, f := range files {
		total += f.S
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		e.fail("Impossible de créer le dossier du jeu (choisis-en un autre).")
		return
	}
	// espace disque (les .azdl déjà téléchargés comptent)
	var partial int64
	for _, f := range files {
		if fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f.P)) + ".azdl"); err == nil && fi.Size() <= f.S {
			partial += fi.Size()
		}
	}
	if free, ok := diskFree(dir); ok && free+uint64(partial) < uint64(total)+(200<<20) {
		e.fail(fmt.Sprintf("Espace disque insuffisant : il faut %s (libre : %s).", humanSize(total-partial), humanSize(int64(free))))
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.cancel = cancel
	e.mu.Unlock()

	// les gros fichiers d'abord : la fin du téléchargement reste fluide
	sort.Slice(files, func(i, j int) bool { return files[i].S > files[j].S })

	var done int64 = partial
	start := time.Now()
	startDone := done
	e.set(func(s *Snapshot) {
		s.Phase, s.Status, s.Busy, s.Progress = PhaseDownloading, "Téléchargement en cours…", true, float64(done)/float64(total)
		s.Detail = humanSize(done) + " / " + humanSize(total)
	})

	// rafraîchissement de l'affichage
	stopUI := make(chan struct{})
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stopUI:
				return
			case <-t.C:
				d := atomic.LoadInt64(&done)
				el := time.Since(start).Seconds()
				speed := float64(d-startDone) / el
				e.set(func(s *Snapshot) {
					s.Progress = float64(d) / float64(total)
					s.Detail = humanSize(d) + " / " + humanSize(total)
					if el > 1 && speed > 0 {
						s.Detail += " — " + humanSize(int64(speed)) + "/s — reste " + humanDuration(float64(total-d)/speed)
					}
				})
			}
		}
	}()

	jobs := make(chan FileEntry)
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
	st := loadState(dir)
	var stMu sync.Mutex
	for i := 0; i < downloadWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				if ctx.Err() != nil {
					continue
				}
				err := downloadFile(ctx, m, dir, f, &done)
				if err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("%s : %v", f.P, err)
					}
					errMu.Unlock()
					cancel()
					continue
				}
				if fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f.P))); err == nil {
					stMu.Lock()
					st[f.P] = stateEntry{S: f.S, M: fi.ModTime().Unix(), H: f.H}
					stMu.Unlock()
				}
			}
		}()
	}
	for _, f := range files {
		if ctx.Err() != nil {
			break
		}
		jobs <- f
	}
	close(jobs)
	wg.Wait()
	close(stopUI)
	saveState(dir, st)
	cancel()
	e.mu.Lock()
	e.cancel = nil
	e.mu.Unlock()

	switch {
	case firstErr != nil:
		e.fail("Téléchargement interrompu : " + firstErr.Error())
	case ctx.Err() != nil && atomic.LoadInt64(&done) < total:
		e.check(m, false)
		e.set(func(s *Snapshot) {
			if s.Phase == PhaseNeedUpdate || s.Phase == PhaseNeedInstall {
				s.Status = "Téléchargement en pause. Il reprendra là où il s'est arrêté."
			}
		})
	default:
		e.check(m, false)
	}
}

// Télécharge un fichier (reprise via .azdl + Range, contrôle MD5, 5 essais).
func downloadFile(ctx context.Context, m *Manifest, dir string, f FileEntry, done *int64) error {
	dst := filepath.Join(dir, filepath.FromSlash(f.P))
	if !strings.HasPrefix(filepath.Clean(dst), filepath.Clean(dir)) {
		return errors.New("chemin refusé")
	}
	os.MkdirAll(filepath.Dir(dst), 0o755)
	part := dst + ".azdl"
	u := fileURL(m, f.P)
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil
		}
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Duration(attempt*3) * time.Second):
			}
		}
		lastErr = downloadOnce(ctx, u, part, f, done)
		if lastErr == nil {
			h, err := md5File(part, nil)
			if err != nil {
				lastErr = err
				continue
			}
			if !strings.EqualFold(h, f.H) {
				atomic.AddInt64(done, -f.S)
				os.Remove(part)
				lastErr = errors.New("fichier corrompu (empreinte différente)")
				continue
			}
			os.Remove(dst)
			if err := os.Rename(part, dst); err != nil {
				return err
			}
			return nil
		}
		if ctx.Err() != nil {
			return nil
		}
	}
	return lastErr
}

func downloadOnce(ctx context.Context, u, part string, f FileEntry, done *int64) error {
	var have int64
	if fi, err := os.Stat(part); err == nil {
		have = fi.Size()
		if have > f.S {
			os.Remove(part)
			atomic.AddInt64(done, -have)
			have = 0
		}
	}
	if have == f.S {
		return nil
	}
	rctx, rcancel := context.WithCancel(ctx)
	defer rcancel()
	req, _ := http.NewRequestWithContext(rctx, "GET", u, nil)
	req.Header.Set("User-Agent", "AzimovLauncher/"+AppVersion)
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	flags := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case 206:
		flags |= os.O_APPEND
	case 200:
		flags |= os.O_TRUNC
		atomic.AddInt64(done, -have) // le serveur renvoie tout : on repart de zéro
		have = 0
	default:
		return fmt.Errorf("erreur HTTP %d", resp.StatusCode)
	}
	out, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	// coupure si rien n'arrive pendant 60 s
	var last atomic.Int64
	last.Store(time.Now().UnixNano())
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if time.Since(time.Unix(0, last.Load())) > stallTimeout {
					rcancel()
					return
				}
			}
		}
	}()

	buf := make([]byte, 256<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
			atomic.AddInt64(done, int64(n))
			last.Store(time.Now().UnixNano())
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			if rctx.Err() != nil && ctx.Err() == nil {
				return errors.New("connexion bloquée")
			}
			return rerr
		}
	}
	if fi, err := out.Stat(); err == nil && fi.Size() != f.S {
		return fmt.Errorf("taille incomplète (%d / %d)", fi.Size(), f.S)
	}
	return nil
}

// ---------------------------------------------------------------- lancement

func (e *Engine) Launch() error {
	e.mu.Lock()
	m := e.manifest
	s := e.settings
	e.mu.Unlock()
	if m == nil {
		return errors.New("manifeste absent")
	}
	dir := s.GameDir
	realm := m.Realmlist
	locale := m.Locale
	if locale == "" {
		locale = "frFR"
	}
	// realmlist.wtf
	rl := filepath.Join(dir, "Data", locale, "realmlist.wtf")
	os.MkdirAll(filepath.Dir(rl), 0o755)
	os.WriteFile(rl, []byte("set realmlist "+realm+"\r\nset patchlist "+realm+"\r\n"), 0o644)
	// WTF\Config.wtf : SET realmList "..."
	setConfigWtf(filepath.Join(dir, "WTF", "Config.wtf"), "realmList", realm)
	if s.ClearCache {
		os.RemoveAll(filepath.Join(dir, "Cache"))
	}
	exe := filepath.Join(dir, filepath.FromSlash(m.Exe))
	cmd := exec.Command(exe)
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return err
	}
	cmd.Process.Release()
	return nil
}

func setConfigWtf(p, key, value string) {
	lines := []string{}
	if b, err := os.ReadFile(p); err == nil {
		for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
			f := strings.Fields(l)
			if len(f) >= 2 && strings.EqualFold(f[0], "SET") && strings.EqualFold(f[1], key) {
				continue
			}
			if strings.TrimSpace(l) != "" {
				lines = append(lines, l)
			}
		}
	}
	lines = append(lines, fmt.Sprintf(`SET %s "%s"`, key, value))
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(strings.Join(lines, "\r\n")+"\r\n"), 0o644)
}

// ---------------------------------------------------------------- mise à jour du launcher

func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// Télécharge le nouveau launcher, remplace l'exe et le relance. true = le launcher va redémarrer.
func (e *Engine) selfUpdate(m *Manifest) bool {
	e.set(func(s *Snapshot) {
		s.Phase, s.Status, s.Detail, s.Busy, s.Progress = PhaseSelfUpdate, "Mise à jour du launcher ("+m.Launcher.Version+")…", "", true, -1
	})
	self := selfPath()
	u, err := url.Parse(ManifestURL)
	if err != nil {
		return false
	}
	u.Path = path.Join(path.Dir(u.Path), m.Launcher.URL)
	tmp := self + ".new"
	os.Remove(tmp)
	var done int64
	if err := downloadOnce(context.Background(), u.String(), tmp, FileEntry{P: m.Launcher.URL, S: m.Launcher.S, H: m.Launcher.H}, &done); err != nil {
		os.Remove(tmp)
		return false
	}
	if h, _ := md5File(tmp, nil); !strings.EqualFold(h, m.Launcher.H) {
		os.Remove(tmp)
		return false
	}
	os.Remove(self + ".old")
	if err := os.Rename(self, self+".old"); err != nil {
		os.Remove(tmp)
		return false
	}
	if err := os.Rename(tmp, self); err != nil {
		os.Rename(self+".old", self)
		return false
	}
	cmd := exec.Command(self)
	cmd.Dir = filepath.Dir(self)
	if cmd.Start() == nil {
		os.Exit(0)
	}
	return false
}
