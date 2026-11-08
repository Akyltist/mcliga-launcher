// mkres создаёт файл ресурсов Windows (rsrc_windows_amd64.syso) для MCLiga.exe:
// сведения о версии (то, что видно в свойствах файла) и манифест.
// Только стандартная библиотека Go, без внешних зависимостей.
//
// Запуск из корня репозитория:  go run ./tools/mkres
// Версия берётся из файла VERSION. Результат попадает в rsrc_windows_amd64.syso,
// который go build подхватывает автоматически.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

const (
	rtVersion  = 16
	rtManifest = 24
	langEN     = 0x0409
)

var verRe = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)

func main() {
	raw, err := os.ReadFile("VERSION")
	if err != nil {
		fail(err)
	}
	ver := strings.TrimSpace(string(raw))
	m := verRe.FindStringSubmatch(ver)
	if m == nil {
		fail(fmt.Errorf("VERSION должен быть вида 1.2.3, а не %q", ver))
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	c, _ := strconv.Atoi(m[3])

	obj := buildObject([]resource{
		{typ: rtVersion, id: 1, data: versionInfo(a, b, c, ver)},
		{typ: rtManifest, id: 1, data: []byte(manifest(ver))},
	})
	if err := os.WriteFile("rsrc_windows_amd64.syso", obj, 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("rsrc_windows_amd64.syso создан (версия %s, %d байт)\n", ver, len(obj))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "ошибка:", err)
	os.Exit(1)
}

// ---------- содержимое ----------

func manifest(ver string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity type="win32" name="ru.mcliga.launcher" version="` + ver + `.0" processorArchitecture="amd64"/>
  <description>MCLiga Launcher</description>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security>
      <requestedPrivileges>
        <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
      </requestedPrivileges>
    </security>
  </trustInfo>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
    <application>
      <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
    </application>
  </compatibility>
</assembly>
`
}

// versionInfo собирает структуру VS_VERSIONINFO.
func versionInfo(a, b, c int, ver string) []byte {
	fileVer := fmt.Sprintf("%s.0", ver)
	strs := [][2]string{
		{"CompanyName", "MCLiga project authors"},
		{"FileDescription", "MCLiga Launcher"},
		{"FileVersion", fileVer},
		{"InternalName", "MCLiga"},
		{"LegalCopyright", "Copyright (c) 2026 MCLiga project authors"},
		{"OriginalFilename", "MCLiga.exe"},
		{"ProductName", "MCLiga Launcher"},
		{"ProductVersion", ver},
	}

	// строки StringTable
	var table bytes.Buffer
	for _, kv := range strs {
		table.Write(block(utf16z(kv[0]), utf16z(kv[1]), len(utf16z(kv[1]))/2, 1, nil))
	}
	stringTable := block(utf16z("040904B0"), nil, 0, 1, table.Bytes())
	stringFileInfo := block(utf16z("StringFileInfo"), nil, 0, 1, stringTable)

	var tr bytes.Buffer
	binary.Write(&tr, binary.LittleEndian, uint32(0x04B00409)) // язык 0x0409, кодовая страница 1200
	varBlock := block(utf16z("Translation"), tr.Bytes(), 4, 0, nil)
	varFileInfo := block(utf16z("VarFileInfo"), nil, 0, 1, varBlock)

	// фиксированная часть
	var ff bytes.Buffer
	w := func(v uint32) { binary.Write(&ff, binary.LittleEndian, v) }
	w(0xFEEF04BD)
	w(0x00010000)
	w(uint32(a)<<16 | uint32(b)) // FileVersionMS
	w(uint32(c)<<16 | 0)         // FileVersionLS
	w(uint32(a)<<16 | uint32(b)) // ProductVersionMS
	w(uint32(c)<<16 | 0)         // ProductVersionLS
	w(0x3F)                      // маска флагов
	w(0)                         // флаги
	w(0x00040004)                // VOS_NT_WINDOWS32
	w(1)                         // VFT_APP
	w(0)
	w(0)
	w(0)

	children := append(append([]byte{}, stringFileInfo...), varFileInfo...)
	return block(utf16z("VS_VERSION_INFO"), ff.Bytes(), ff.Len(), 0, children)
}

func utf16z(s string) []byte {
	u := utf16.Encode([]rune(s))
	u = append(u, 0)
	out := make([]byte, len(u)*2)
	for i, v := range u {
		binary.LittleEndian.PutUint16(out[i*2:], v)
	}
	return out
}

func pad4(b *bytes.Buffer) {
	for b.Len()%4 != 0 {
		b.WriteByte(0)
	}
}

// block — общий вид узла версии: длина, длина значения, тип, ключ, значение, дочерние узлы.
// valueLen: для текстовых значений число символов (слов), для двоичных число байт.
func block(key, value []byte, valueLen, typ int, children []byte) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, uint16(0)) // длина впишем позже
	binary.Write(&b, binary.LittleEndian, uint16(valueLen))
	binary.Write(&b, binary.LittleEndian, uint16(typ))
	b.Write(key)
	pad4(&b)
	b.Write(value)
	pad4(&b)
	if len(children) > 0 {
		b.Write(children)
		pad4(&b)
	}
	out := b.Bytes()
	binary.LittleEndian.PutUint16(out[0:], uint16(len(out)))
	return out
}

// ---------- объектный файл COFF ----------

type resource struct {
	typ, id int
	data    []byte
}

func buildObject(res []resource) []byte {
	le := binary.LittleEndian
	// Структура раздела .rsrc: корневой каталог (типы) -> каталоги имён -> каталоги языков -> записи данных -> данные.
	n := len(res)
	rootSize := 16 + 8*n
	nameDirSize := 16 + 8
	langDirSize := 16 + 8
	offNameDirs := rootSize
	offLangDirs := offNameDirs + n*nameDirSize
	offEntries := offLangDirs + n*langDirSize
	offData := offEntries + n*16

	var sec bytes.Buffer
	dirHeader := func(ids int) {
		binary.Write(&sec, le, uint32(0)) // Characteristics
		binary.Write(&sec, le, uint32(0)) // TimeDateStamp
		binary.Write(&sec, le, uint16(0)) // Major
		binary.Write(&sec, le, uint16(0)) // Minor
		binary.Write(&sec, le, uint16(0)) // именованные записи
		binary.Write(&sec, le, uint16(ids))
	}
	// корень: типы по возрастанию
	dirHeader(n)
	for i, r := range res {
		binary.Write(&sec, le, uint32(r.typ))
		binary.Write(&sec, le, uint32(0x80000000|uint32(offNameDirs+i*nameDirSize)))
	}
	for i, r := range res {
		dirHeader(1)
		binary.Write(&sec, le, uint32(r.id))
		binary.Write(&sec, le, uint32(0x80000000|uint32(offLangDirs+i*langDirSize)))
	}
	for i := range res {
		dirHeader(1)
		binary.Write(&sec, le, uint32(langEN))
		binary.Write(&sec, le, uint32(offEntries+i*16))
	}
	// записи данных и сами данные
	var data bytes.Buffer
	var relocs []uint32 // смещения полей OffsetToData
	cur := offData
	for i, r := range res {
		binary.Write(&sec, le, uint32(cur)) // смещение внутри раздела; прибавит компоновщик по перемещению
		binary.Write(&sec, le, uint32(len(r.data)))
		binary.Write(&sec, le, uint32(0))
		binary.Write(&sec, le, uint32(0))
		relocs = append(relocs, uint32(offEntries+i*16))
		data.Write(r.data)
		for data.Len()%4 != 0 {
			data.WriteByte(0)
		}
		cur = offData + data.Len()
	}
	sec.Write(data.Bytes())

	// сборка COFF
	const hdr = 20 + 40
	rawSize := sec.Len()
	relOff := hdr + rawSize
	symOff := relOff + len(relocs)*10

	var out bytes.Buffer
	// заголовок файла
	binary.Write(&out, le, uint16(0x8664)) // AMD64
	binary.Write(&out, le, uint16(1))      // разделов
	binary.Write(&out, le, uint32(0))
	binary.Write(&out, le, uint32(symOff))
	binary.Write(&out, le, uint32(1)) // символов
	binary.Write(&out, le, uint16(0))
	binary.Write(&out, le, uint16(0))
	// заголовок раздела
	out.Write([]byte{'.', 'r', 's', 'r', 'c', 0, 0, 0})
	binary.Write(&out, le, uint32(0)) // VirtualSize
	binary.Write(&out, le, uint32(0)) // VirtualAddress
	binary.Write(&out, le, uint32(rawSize))
	binary.Write(&out, le, uint32(hdr))
	binary.Write(&out, le, uint32(relOff))
	binary.Write(&out, le, uint32(0))
	binary.Write(&out, le, uint16(len(relocs)))
	binary.Write(&out, le, uint16(0))
	binary.Write(&out, le, uint32(0x40000040)) // данные, только чтение
	out.Write(sec.Bytes())
	// перемещения: IMAGE_REL_AMD64_ADDR32NB
	for _, r := range relocs {
		binary.Write(&out, le, r)
		binary.Write(&out, le, uint32(0)) // символ .rsrc
		binary.Write(&out, le, uint16(3))
	}
	// таблица символов: один символ — сам раздел
	out.Write([]byte{'.', 'r', 's', 'r', 'c', 0, 0, 0})
	binary.Write(&out, le, uint32(0))
	binary.Write(&out, le, uint16(1)) // номер раздела
	binary.Write(&out, le, uint16(0))
	out.WriteByte(3) // STATIC
	out.WriteByte(0) // без дополнительных записей
	// таблица строк (пустая)
	binary.Write(&out, le, uint32(4))
	return out.Bytes()
}
