#!/usr/bin/env python3
"""Génère rsrc_windows_amd64.syso (icône + manifeste + version) pour `go build`.

Usage : python tools/mkrsrc.py   (depuis le dossier du launcher)
Le fichier .syso est déjà fourni dans le dépôt : ne relancer que si l'icône,
le manifeste ou la version changent.
"""
import re
import struct
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
ICO = ROOT / "assets" / "azimov.ico"
MANIFEST = ROOT / "assets" / "app.manifest"
OUT = ROOT / "rsrc_windows_amd64.syso"

RT_ICON, RT_GROUP_ICON, RT_VERSION, RT_MANIFEST = 3, 14, 16, 24
LANG = 0x040C  # français (France)


def app_version():
    src = (ROOT / "core.go").read_text(encoding="utf-8")
    m = re.search(r'const AppVersion = "([0-9.]+)"', src)
    parts = [int(x) for x in m.group(1).split(".")] if m else [1, 0, 0]
    return (parts + [0, 0, 0, 0])[:4], m.group(1) if m else "1.0.0"


def read_ico(p):
    d = p.read_bytes()
    _, _, n = struct.unpack_from("<HHH", d, 0)
    images, group = [], b""
    for i in range(n):
        w, h, cc, r, planes, bpp, size, off = struct.unpack_from("<BBBBHHII", d, 6 + 16 * i)
        images.append(d[off:off + size])
        group += struct.pack("<BBBBHHIH", w, h, cc, r, planes, bpp, size, i + 1)
    return images, struct.pack("<HHH", 0, 1, n) + group


def vs_string(key, value):
    k = (key + "\0").encode("utf-16le")
    v = (value + "\0").encode("utf-16le")
    hdr_len = 6 + len(k)
    pad = (4 - hdr_len % 4) % 4
    body = k + b"\0" * pad + v
    return struct.pack("<HHH", 6 + len(body), len(v) // 2, 1) + body


def vs_block(key, children, value=b"", vtype=1, vlen=None):
    k = (key + "\0").encode("utf-16le")
    out = k + b"\0" * ((4 - (6 + len(k)) % 4) % 4) + value
    out += b"\0" * ((4 - (6 + len(out)) % 4) % 4)
    for c in children:
        out += c + b"\0" * ((4 - len(c) % 4) % 4)
    vl = (len(value) if vlen is None else vlen)
    return struct.pack("<HHH", 6 + len(out), vl, vtype) + out


def version_resource():
    v, vs = app_version()
    ms, ls = (v[0] << 16) | v[1], (v[2] << 16) | v[3]
    ffi = struct.pack("<13I", 0xFEEF04BD, 0x00010000, ms, ls, ms, ls, 0x3F, 0, 0x4, 0x1, 0, 0, 0)
    strings = [vs_string(k, val) for k, val in (
        ("CompanyName", "Azimov Online"),
        ("FileDescription", "Azimov Online Launcher"),
        ("FileVersion", vs),
        ("InternalName", "AzimovLauncher"),
        ("OriginalFilename", "AzimovLauncher.exe"),
        ("ProductName", "Azimov Online"),
        ("ProductVersion", vs),
    )]
    st = vs_block("040C04B0", strings)
    sfi = vs_block("StringFileInfo", [st])
    var = vs_block("Translation", [], struct.pack("<HH", LANG, 1200), vtype=0)
    vfi = vs_block("VarFileInfo", [var])
    return vs_block("VS_VERSION_INFO", [sfi, vfi], ffi, vtype=0)


def build():
    images, group = read_ico(ICO)
    res = {RT_ICON: [(i + 1, img) for i, img in enumerate(images)],
           RT_GROUP_ICON: [(1, group)],
           RT_VERSION: [(1, version_resource())],
           RT_MANIFEST: [(1, MANIFEST.read_bytes())]}
    types = sorted(res)

    # --- arborescence : type -> nom -> langue -> données
    dir_size = lambda n: 16 + 8 * n
    off = dir_size(len(types))
    type_dirs = {}
    for t in types:
        type_dirs[t] = off
        off += dir_size(len(res[t]))
    name_dirs = {}
    for t in types:
        for nid, _ in res[t]:
            name_dirs[(t, nid)] = off
            off += dir_size(1)
    data_entries = {}
    for t in types:
        for nid, _ in res[t]:
            data_entries[(t, nid)] = off
            off += 16
    data_off = {}
    for t in types:
        for nid, blob in res[t]:
            off = (off + 7) & ~7
            data_off[(t, nid)] = off
            off += len(blob)
    total = (off + 7) & ~7

    sec = bytearray(total)

    def put_dir(pos, entries):
        struct.pack_into("<IIHHHH", sec, pos, 0, 0, 0, 0, 0, len(entries))
        for i, (ident, target, is_dir) in enumerate(entries):
            struct.pack_into("<II", sec, pos + 16 + 8 * i, ident, target | (0x80000000 if is_dir else 0))

    put_dir(0, [(t, type_dirs[t], True) for t in types])
    for t in types:
        put_dir(type_dirs[t], [(nid, name_dirs[(t, nid)], True) for nid, _ in res[t]])
        for nid, _ in res[t]:
            put_dir(name_dirs[(t, nid)], [(LANG, data_entries[(t, nid)], False)])
    relocs = []
    for t in types:
        for nid, blob in res[t]:
            de = data_entries[(t, nid)]
            struct.pack_into("<IIII", sec, de, data_off[(t, nid)], len(blob), 0, 0)
            relocs.append(de)  # OffsetToData : RVA à corriger par l'éditeur de liens
            sec[data_off[(t, nid)]:data_off[(t, nid)] + len(blob)] = blob

    # --- objet COFF (amd64) avec une section .rsrc
    hdr_size, sh_size = 20, 40
    raw_ptr = hdr_size + sh_size
    reloc_ptr = raw_ptr + len(sec)
    sym_ptr = reloc_ptr + 10 * len(relocs)
    coff = struct.pack("<HHIIIHH", 0x8664, 1, 0, sym_ptr, 1, 0, 0)
    shdr = struct.pack("<8sIIIIIIHHI", b".rsrc\0\0\0", 0, 0, len(sec), raw_ptr, reloc_ptr, 0,
                       len(relocs), 0, 0x40000040)  # INITIALIZED_DATA | READ
    rel = b"".join(struct.pack("<IIH", r, 0, 3) for r in relocs)  # IMAGE_REL_AMD64_ADDR32NB
    sym = struct.pack("<8sIhHBB", b".rsrc\0\0\0", 0, 1, 0, 3, 0)  # symbole de section (static)
    strtab = struct.pack("<I", 4)
    OUT.write_bytes(coff + shdr + bytes(sec) + rel + sym + strtab)
    print(f"{OUT.name} : {len(images)} icônes, manifeste, version {app_version()[1]}")


if __name__ == "__main__":
    sys.exit(build())
