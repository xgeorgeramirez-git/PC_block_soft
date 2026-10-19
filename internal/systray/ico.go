package systray

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
)

// IconSides — стороны значка в ICO: 100, 150 и 200 % масштаба Windows.
var IconSides = []int{size100, size150, size200}

// PickSide выбирает из IconSides ближайшую сторону не меньше нужной: чуть больший значок
// Windows ужимает чище, чем растягивает меньший.
func PickSide(px int) int {
	for _, s := range IconSides {
		if px <= s {
			return s
		}
	}
	return IconSides[len(IconSides)-1]
}

// BuildICO рисует значок во всех размерах и упаковывает в ICO.
func BuildICO(label string, st Status) ([]byte, error) {
	imgs := make([]image.Image, 0, len(IconSides))
	for _, side := range IconSides {
		imgs = append(imgs, RenderIcon(label, st, side))
	}
	return EncodeICO(imgs...)
}

// ICOImage достаёт из файла ICO (как его пишет EncodeICO) данные картинки нужной стороны:
// для CreateIconFromResourceEx, которому нужна одна картинка, а не весь файл.
func ICOImage(ico []byte, side int) ([]byte, error) {
	const head, entry = 6, 16
	if len(ico) < head {
		return nil, errors.New("systray: ICO короче заголовка")
	}
	le := binary.LittleEndian
	if le.Uint16(ico[0:]) != 0 || le.Uint16(ico[2:]) != 1 {
		return nil, errors.New("systray: это не файл значка ICO")
	}
	n := int(le.Uint16(ico[4:]))
	if len(ico) < head+entry*n {
		return nil, errors.New("systray: каталог ICO обрезан")
	}
	for i := 0; i < n; i++ {
		e := ico[head+entry*i : head+entry*(i+1)]
		w := int(e[0])
		if w == 0 {
			w = icoMaxSide
		}
		if w != side {
			continue
		}
		size, off := int(le.Uint32(e[8:])), int(le.Uint32(e[12:]))
		if off < 0 || size <= 0 || off+size > len(ico) {
			return nil, fmt.Errorf("systray: картинка %d×%d в ICO выходит за файл", side, side)
		}
		return ico[off : off+size], nil
	}
	return nil, fmt.Errorf("systray: в ICO нет картинки %d×%d", side, side)
}
