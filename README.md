# Azimov Online — Launcher

Launcher Windows du serveur **Azimov Online** (World of Warcraft Cataclysm 4.3.4).

- Installe le client complet et le patch du serveur, puis le garde à jour (téléchargement sur 3 connexions, reprise après coupure, contrôle MD5).
- Vérification rapide au démarrage (état mémorisé dans `<jeu>\.azimov\etat.json`) ; « Vérifier les fichiers » recalcule toutes les empreintes (journal : `<jeu>\.azimov\verification.log`).
- Règle la realmlist (`Data\frFR\realmlist.wtf` + `WTF\Config.wtf`), vide le cache si la case est cochée, lance le jeu.
- Statut du royaume, joueurs connectés et actualités (lus dans `api.php` du site).
- Liens **Site web**, **Voter** et **Créer un compte**.
- Se met à jour tout seul quand une nouvelle version est déposée sur le serveur.

Écrit en Go, sans aucune dépendance (Win32 + GDI+ appelés directement) : un seul `.exe` d'environ 7 Mo.

## Compiler

Il faut [Go](https://go.dev/dl/) 1.22 ou plus récent.

```bat
build.bat
```

ou à la main :

```bat
go build -trimpath -ldflags "-H windowsgui -s -w" -o AzimovLauncher.exe .
```

Depuis Linux / macOS : `GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o AzimovLauncher.exe .`

L'icône, le manifeste Windows et les infos de version sont dans `rsrc_windows_amd64.syso` (pris en compte automatiquement par `go build`).
Après un changement d'icône (`assets/azimov.ico`), de manifeste (`assets/app.manifest`) ou de version (`AppVersion` dans `core.go`) :

```bat
python tools\mkrsrc.py
```

Tests du cœur (téléchargement, reprise, vérification) : `go test ./...`

## Configuration

Adresses par défaut dans `core.go` (`ManifestURL`, `APIURL`, `SiteURL`). Elles peuvent être changées sans recompiler avec un fichier `launcher.ini` posé à côté de l'exe :

```ini
manifest=http://mon-serveur/launcher/manifest.json
api=http://mon-serveur/api.php
site=http://mon-serveur/
vote=http://mon-serveur/voter.php
register=http://mon-serveur/inscription.php
```

Réglages du joueur : `%APPDATA%\AzimovLauncher\settings.json` (dossier du jeu, vider le cache).

## Côté serveur (XAMPP)

```
htdocs/
  api.php                      statut + actualités (JSON) : server, realm, auth, world, online, realmlist, site, register, vote, news[]
  launcher/
    AzimovLauncher.exe         le launcher distribué (et sa mise à jour automatique)
    manifest.json              généré par generer_manifest.php
    generer_manifest.php       C:\xampp\php\php.exe C:\xampp\htdocs\launcher\generer_manifest.php
    client/                    le client 4.3.4 complet déjà patché
```

Format de `manifest.json` :

```json
{"name":"Azimov Online","version":"1","exe":"Azimov1.0.exe","realmlist":"163.172.102.172","locale":"frFR",
 "base":"client/","total":18738753398,"delete":[],
 "launcher":{"version":"1.1.0","url":"AzimovLauncher.exe","h":"<md5>","s":7000000},
 "files":[{"p":"Data/art.MPQ","s":3943419041,"h":"<md5>"}]}
```

### Publier une nouvelle version du launcher

1. Augmenter `AppVersion` dans `core.go`, lancer `python tools\mkrsrc.py`, compiler.
2. Copier `AzimovLauncher.exe` dans `htdocs\launcher\`.
3. Mettre la même version dans `$LAUNCHER_VERSION` de `generer_manifest.php` et relancer le script.

Au prochain démarrage, chaque launcher plus ancien télécharge le nouveau, le vérifie (MD5) et redémarre dessus.

## Fichiers

| Fichier | Rôle |
|---|---|
| `main_windows.go` | démarrage (DPI, GDI+, fenêtre, boucle de messages) |
| `ui_windows.go` | dessin de la fenêtre, zones cliquables, souris |
| `gfx_windows.go` | petite couche GDI+ (images, polices embarquées, formes, texte) |
| `win32_windows.go` | liaisons Win32 / GDI+ |
| `core.go` | manifeste, vérification, téléchargement, lancement, mise à jour |
| `assets/` | fond, logo, icône, polices, manifeste Windows |
| `tools/mkrsrc.py` | génère `rsrc_windows_amd64.syso` |

## Crédits

Polices **Cinzel** (Natanael Gama) et **Marcellus** (Brian J. Bonislawsky), sous licence SIL Open Font License 1.1.
World of Warcraft® est une marque de Blizzard Entertainment ; ce projet n'y est pas lié.
