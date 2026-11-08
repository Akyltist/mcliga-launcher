package main

// Опрос игрового сервера по протоколу Minecraft (Server List Ping).
// Время между отправкой запроса статуса и получением ответа и есть пинг.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"time"
)

func writeVarInt(b *bytes.Buffer, v int) {
	u := uint32(v)
	for {
		if u&^0x7f == 0 {
			b.WriteByte(byte(u))
			return
		}
		b.WriteByte(byte(u&0x7f | 0x80))
		u >>= 7
	}
}

func readVarInt(r io.ByteReader) (int, error) {
	var res uint32
	for i := 0; i < 5; i++ {
		c, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		res |= uint32(c&0x7f) << (7 * uint(i))
		if c&0x80 == 0 {
			return int(res), nil
		}
	}
	return 0, errors.New("слишком длинное число")
}

func packet(id int, body []byte) []byte {
	var p bytes.Buffer
	writeVarInt(&p, id)
	p.Write(body)
	var out bytes.Buffer
	writeVarInt(&out, p.Len())
	out.Write(p.Bytes())
	return out.Bytes()
}

type serverStatus struct {
	Online, Max int
	PingMS      int
}

// queryServer возвращает онлайн сервера и время ответа в миллисекундах.
func queryServer(host string, port int) (serverStatus, error) {
	var st serverStatus
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, itoa(port)), 4*time.Second)
	if err != nil {
		return st, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(6 * time.Second))

	var hs bytes.Buffer
	writeVarInt(&hs, 767) // версия протокола: сервер отвечает на статус при любой
	writeVarInt(&hs, len(host))
	hs.WriteString(host)
	binary.Write(&hs, binary.BigEndian, uint16(port))
	writeVarInt(&hs, 1) // следующее состояние: статус
	conn.Write(packet(0x00, hs.Bytes()))

	start := time.Now()
	conn.Write(packet(0x00, nil)) // запрос статуса
	br := &byteReader{c: conn}
	length, err := readVarInt(br)
	if err != nil {
		return st, err
	}
	_ = start
	if length <= 0 || length > 1<<20 {
		return st, errors.New("странный ответ сервера")
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(br, buf); err != nil {
		return st, err
	}
	rd := bytes.NewReader(buf)
	if _, err := readVarInt(rd); err != nil { // номер пакета
		return st, err
	}
	n, err := readVarInt(rd)
	if err != nil || n <= 0 || n > rd.Len() {
		return st, errors.New("странный ответ сервера")
	}
	js := make([]byte, n)
	io.ReadFull(rd, js)
	var s struct {
		Players struct {
			Online int `json:"online"`
			Max    int `json:"max"`
		} `json:"players"`
	}
	if err := json.Unmarshal(js, &s); err != nil {
		return st, err
	}
	st.Online, st.Max = s.Players.Online, s.Players.Max

	// пинг: второй обмен на том же соединении (без времени установки соединения)
	var payload [8]byte
	binary.BigEndian.PutUint64(payload[:], uint64(time.Now().UnixNano()))
	t2 := time.Now()
	conn.Write(packet(0x01, payload[:]))
	if l2, err := readVarInt(br); err == nil && l2 > 0 && l2 < 64 {
		io.ReadFull(br, make([]byte, l2))
		st.PingMS = int(time.Since(t2).Milliseconds())
	} else {
		st.PingMS = -1
	}
	return st, nil
}

type byteReader struct{ c net.Conn }

func (b *byteReader) ReadByte() (byte, error) {
	var one [1]byte
	_, err := io.ReadFull(b.c, one[:])
	return one[0], err
}
func (b *byteReader) Read(p []byte) (int, error) { return b.c.Read(p) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for ; n > 0; n /= 10 {
		d = append([]byte{byte('0' + n%10)}, d...)
	}
	return string(d)
}
