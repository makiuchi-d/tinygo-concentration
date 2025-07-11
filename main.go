package main

import (
	"fmt"
	"image/color"
	"machine"
	"math/rand"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/encoders"
	"tinygo.org/x/drivers/ssd1306"
	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/proggy"
)

type WS2812B struct {
	*piolib.WS2812B
	Pin machine.Pin
}

func NewWS2812B(pin machine.Pin) *WS2812B {
	s, _ := pio.PIO0.ClaimStateMachine()
	ws, _ := piolib.NewWS2812B(s, pin)
	ws.EnableDMA(true)
	return &WS2812B{WS2812B: ws}
}

type card struct {
	color   uint32
	removed bool
	open    bool
}

type cards []card

func newCards(hard bool) cards {
	n1, _ := machine.GetRNG()
	n2, _ := machine.GetRNG()
	rng := rand.New(rand.NewSource(int64(n1)<<32 + int64(n2)))

	var cs cards
	if hard {
		cs = make([]card, 12)
		for i := range 6 {
			clr := randColor(rng)
			cs[i*2].color = clr
			cs[i*2+1].color = clr
		}
	} else {
		cs = cards{
			card{color: 0xff0000ff}, card{color: 0xff0000ff},
			card{color: 0x00ff00ff}, card{color: 0x00ff00ff},
			card{color: 0x0000ffff}, card{color: 0x0000ffff},
			card{color: 0xffff00ff}, card{color: 0xffff00ff},
			card{color: 0xff00ffff}, card{color: 0xff00ffff},
			card{color: 0x00ffffff}, card{color: 0x00ffffff},
		}
	}

	rng.Shuffle(len(cs), func(i, j int) {
		cs[i], cs[j] = cs[j], cs[i]
	})
	return cs
}

func randColor(rng *rand.Rand) uint32 {
	for {
		c := rng.Uint32() & 0xf0f0f000
		c = c | (c >> 8) | 0x000000ff
		if c != 0x000000ff && c != 0xffffffff {
			return c
		}
	}
}

func (cs cards) getRaw() []uint32 {
	r := make([]uint32, 12)
	for i := range 12 {
		if cs[i].removed {
			r[i] = 0x00000000
			continue
		}
		if !cs[i].open {
			r[i] = 0xffffffff
			continue
		}
		r[i] = cs[i].color
	}
	return r
}

var (
	colPins = []machine.Pin{
		machine.GPIO5,
		machine.GPIO6,
		machine.GPIO7,
		machine.GPIO8,
	}
	rowPins = []machine.Pin{
		machine.GPIO9,
		machine.GPIO10,
		machine.GPIO11,
	}
)

func initPins() {
	for _, p := range colPins {
		p.Configure(machine.PinConfig{Mode: machine.PinOutput})
		p.Low()
	}
	for _, p := range rowPins {
		p.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	}
}

func waitKey() int {
	for {
		for i := range 4 {
			for c, p := range colPins {
				if c == i {
					p.High()
				} else {
					p.Low()
				}
			}
			time.Sleep(1 * time.Millisecond)
			for r, p := range rowPins {
				if p.Get() {
					return i*3 + r
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitCloseKey(cs cards) int {
	for {
		p := waitKey()
		if cs[p].removed || cs[p].open {
			continue
		}
		return p
	}
}

type RotatedDisplay struct {
	drivers.Displayer
}

func (d *RotatedDisplay) SetPixel(x, y int16, c color.RGBA) {
	// 180度回転
	sx, sy := d.Displayer.Size()
	d.Displayer.SetPixel(sx-x, sy-y, c)
}

func getSelectInput(rotenc *encoders.QuadratureDevice, rotbtn machine.Pin) bool {
	pos := rotenc.Position()
	for {
		time.Sleep(10 * time.Millisecond)

		if !rotbtn.Get() {
			return true
		}
		if pos != rotenc.Position() {
			return false
		}
	}
}

func main() {
	machine.I2C0.Configure(machine.I2CConfig{
		Frequency: 2.8 * machine.MHz,
		SDA:       machine.GPIO12,
		SCL:       machine.GPIO13,
	})

	display := ssd1306.NewI2C(machine.I2C0)
	display.Configure(ssd1306.Config{
		Address: 0x3C,
		Width:   128,
		Height:  64,
	})
	display.ClearDisplay()

	rotDisplay := RotatedDisplay{&display}
	white := color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	black := color.RGBA{R: 0, G: 0, B: 0, A: 0xFF}

	rotbtn := machine.GPIO2
	rotbtn.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	rotenc := encoders.NewQuadratureViaInterrupt(
		machine.GPIO3,
		machine.GPIO4,
	)
	rotenc.Configure(encoders.QuadratureConfig{
		Precision: 4,
	})

	initPins()
	ws := NewWS2812B(machine.GPIO1)
	ws.WriteRaw([]uint32{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	time.Sleep(time.Second / 2)

	tinyfont.WriteLine(&rotDisplay, &proggy.TinySZ8pt7b, 5, 15, "Select Mode", white)
	hardmode := false
	for {
		if hardmode {
			tinyfont.WriteLine(&rotDisplay, &proggy.TinySZ8pt7b, 5, 30, ">", black)
			tinyfont.WriteLine(&rotDisplay, &proggy.TinySZ8pt7b, 5, 30, "  Normal", white)
			tinyfont.WriteLine(&rotDisplay, &proggy.TinySZ8pt7b, 5, 45, "> Hard", white)
		} else {
			tinyfont.WriteLine(&rotDisplay, &proggy.TinySZ8pt7b, 5, 30, "> Normal", white)
			tinyfont.WriteLine(&rotDisplay, &proggy.TinySZ8pt7b, 5, 45, ">", black)
			tinyfont.WriteLine(&rotDisplay, &proggy.TinySZ8pt7b, 5, 45, "  Hard", white)
		}
		display.Display()

		if getSelectInput(rotenc, rotbtn) {
			fmt.Println("input!")
			break
		}
		fmt.Println("change!")
		hardmode = !hardmode
	}

	time.Sleep(10 * time.Millisecond)

	cs := newCards(hardmode)

	num := 6

	for {
		ws.WriteRaw(cs.getRaw())

		// first card
		p1 := waitCloseKey(cs)
		cs[p1].open = true
		ws.WriteRaw(cs.getRaw())

		// second card
		p2 := waitCloseKey(cs)
		cs[p2].open = true
		ws.WriteRaw(cs.getRaw())

		if cs[p1].color == cs[p2].color {
			time.Sleep(time.Second / 2)
			cs[p1].removed = true
			cs[p2].removed = true
			ws.WriteRaw(cs.getRaw())
			time.Sleep(time.Second / 2)
			cs[p1].removed = false
			cs[p2].removed = false
			ws.WriteRaw(cs.getRaw())
			time.Sleep(time.Second / 2)
			cs[p1].removed = true
			cs[p2].removed = true
			ws.WriteRaw(cs.getRaw())
			time.Sleep(time.Second / 2)
			cs[p1].removed = false
			cs[p2].removed = false
			ws.WriteRaw(cs.getRaw())
			time.Sleep(time.Second / 2)
			cs[p1].removed = true
			cs[p2].removed = true

			num--
			if num == 0 {
				break
			}
		} else {
			time.Sleep(time.Second * 2)
			cs[p1].open = false
			cs[p2].open = false
		}
	}

	ws.WriteRaw(cs.getRaw())
	time.Sleep(time.Second / 2)

	for n := range cs {
		cs[n].removed = false
		cs[n].open = true
	}
	ws.WriteRaw(cs.getRaw())
}
