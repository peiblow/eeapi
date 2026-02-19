package swp

import (
	"encoding/binary"
	"encoding/json"
	"io"
)

func Encode(w io.Writer, msg WireMesage) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}

	frameLen := uint32(len(payload))
	if err := binary.Write(w, binary.BigEndian, uint32(frameLen)); err != nil {
		return err
	}

	_, err = w.Write(payload)
	return err
}

func Decode(r io.Reader, out any) error {
	var frameLen uint32
	if err := binary.Read(r, binary.BigEndian, &frameLen); err != nil {
		return err
	}

	payload := make([]byte, frameLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return err
	}

	return json.Unmarshal(payload, out)
}
