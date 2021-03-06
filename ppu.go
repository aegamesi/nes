package main

import (
	"fmt"
)

type Ppu struct {
	nes *Nes
	mem Memory

	// drawing interfaces
	funcPushPixel func(int, int, color)
	funcPushFrame func()

	vram          [2048]byte
	oam           [256]byte
	secondary_oam [32]byte
	palette       [32]byte
	colors        [64]color

	warmupRemaining int
	scanlineCounter int
	tickCounter     int
	frameCounter    int
	cycles          uint64

	status_rendering    bool
	flag_vBlank         byte
	flag_sprite0Hit     byte
	flag_spriteOverflow byte

	// etc.
	ppuDataBuffer byte

	// scrolling / internal registers
	ppuLatch             byte
	v                    uint16
	t                    uint16
	x                    byte
	w                    byte
	backgroundBitmapData uint64

	// sprite rendering
	spriteEvaluationN         int
	spriteEvaluationM         int
	spriteEvaluationRead      byte
	pendingNumScanlineSprites int
	numScanlineSprites        int
	spriteXPositions          [8]int
	spriteAttributes          [8]byte
	spriteBitmapDataLo        [8]byte
	spriteBitmapDataHi        [8]byte
	spriteZeroAt              int
	spriteZeroAtNext          int

	// PPUCTRL
	flag_baseNametable          byte
	flag_incrementVram          byte
	flag_spriteTableAddress     byte
	flag_backgroundTableAddress byte
	flag_spriteSize             byte
	flag_masterSlave            byte
	flag_generateNMIs           byte

	// PPUMASK
	flag_grayscale          byte
	flag_showSpritesLeft    byte
	flag_showBackgroundLeft byte
	flag_renderSprites      byte
	flag_renderBackground   byte
	flag_emphasizeRed       byte
	flag_emphasizeGreen     byte
	flag_emphasizeBlue      byte

	oamAddr byte
}

func NewPpu(nes *Nes) *Ppu {
	fmt.Println("...")
	return &Ppu{
		nes:             nes,
		mem:             &PPUMemory{nes: nes},
		warmupRemaining: 29658 * 3,
		scanlineCounter: 0, // counts scanlines in a frame ( https://wiki.nesdev.com/w/index.php/PPU_rendering#Line-by-line_timing )
		tickCounter:     0, // counts clock cycle ticks in a scanline
		frameCounter:    0, // counts total frames (vblanks)

		flag_vBlank: 0,
		colors:      [64]color{84*256*256 + 84*256 + 84, 0*256*256 + 30*256 + 116, 8*256*256 + 16*256 + 144, 48*256*256 + 0*256 + 136, 68*256*256 + 0*256 + 100, 92*256*256 + 0*256 + 48, 84*256*256 + 4*256 + 0, 60*256*256 + 24*256 + 0, 32*256*256 + 42*256 + 0, 8*256*256 + 58*256 + 0, 0*256*256 + 64*256 + 0, 0*256*256 + 60*256 + 0, 0*256*256 + 50*256 + 60, 0*256*256 + 0*256 + 0, 0*256*256 + 0*256 + 0, 0*256*256 + 0*256 + 0, 152*256*256 + 150*256 + 152, 8*256*256 + 76*256 + 196, 48*256*256 + 50*256 + 236, 92*256*256 + 30*256 + 228, 136*256*256 + 20*256 + 176, 160*256*256 + 20*256 + 100, 152*256*256 + 34*256 + 32, 120*256*256 + 60*256 + 0, 84*256*256 + 90*256 + 0, 40*256*256 + 114*256 + 0, 8*256*256 + 124*256 + 0, 0*256*256 + 118*256 + 40, 0*256*256 + 102*256 + 120, 0*256*256 + 0*256 + 0, 0*256*256 + 0*256 + 0, 0*256*256 + 0*256 + 0, 236*256*256 + 238*256 + 236, 76*256*256 + 154*256 + 236, 120*256*256 + 124*256 + 236, 176*256*256 + 98*256 + 236, 228*256*256 + 84*256 + 236, 236*256*256 + 88*256 + 180, 236*256*256 + 106*256 + 100, 212*256*256 + 136*256 + 32, 160*256*256 + 170*256 + 0, 116*256*256 + 196*256 + 0, 76*256*256 + 208*256 + 32, 56*256*256 + 204*256 + 108, 56*256*256 + 180*256 + 204, 60*256*256 + 60*256 + 60, 0*256*256 + 0*256 + 0, 0*256*256 + 0*256 + 0, 236*256*256 + 238*256 + 236, 168*256*256 + 204*256 + 236, 188*256*256 + 188*256 + 236, 212*256*256 + 178*256 + 236, 236*256*256 + 174*256 + 236, 236*256*256 + 174*256 + 212, 236*256*256 + 180*256 + 176, 228*256*256 + 196*256 + 144, 204*256*256 + 210*256 + 120, 180*256*256 + 222*256 + 120, 168*256*256 + 226*256 + 144, 152*256*256 + 226*256 + 180, 160*256*256 + 214*256 + 228, 160*256*256 + 162*256 + 160, 0*256*256 + 0*256 + 0, 0*256*256 + 0*256 + 0},
	}
}

func (ppu *Ppu) ReadRegister(register int) byte {
	switch register {
	case 2:
		// PPUSTATUS
		var status byte = ppu.ppuLatch & 0x1F
		status |= ppu.flag_spriteOverflow << 5
		status |= ppu.flag_sprite0Hit << 6
		status |= ppu.flag_vBlank << 7

		ppu.flag_vBlank = 0
		ppu.ppuLatch = status
		ppu.w = 0
		return status
	case 4:
		// OAMDATA
		// TODO if visible scanline and cycle between 1-64, return 0xFF
		return ppu.oam[ppu.oamAddr]
		// XXX increment after read during rendering?
	case 7:
		// PPUDATA
		var data byte
		if ppu.v <= 0x3EFF {
			// buffer this read
			data = ppu.mem.Read(address(ppu.v))
			ppu.ppuDataBuffer, data = data, ppu.ppuDataBuffer
		} else {
			ppu.ppuDataBuffer = ppu.mem.Read(address(ppu.v - 0x1000))
		}

		// fmt.Printf("read PPUDATA: $%.4X (got $%.2X) | PC: $%.4X\n", ppu.v, data, nes.cpu.PC)
		if ppu.flag_incrementVram == 0 {
			ppu.v += 1
		} else {
			ppu.v += 32
		}
		return data
	default:
		return ppu.ppuLatch
	}
}

func (ppu *Ppu) WriteRegister(register int, data byte) {
	ppu.ppuLatch = data
	switch register {
	case 0:
		// PPUCTRL
		if ppu.cycles > 29658*3 {
			ppu.flag_baseNametable = data & 0x3
			ppu.flag_incrementVram = data & 0x4 >> 2
			ppu.flag_spriteTableAddress = data & 0x8 >> 3
			ppu.flag_backgroundTableAddress = data & 0x10 >> 4
			ppu.flag_spriteSize = data & 0x20 >> 5
			ppu.flag_masterSlave = data & 0x40 >> 6
			ppu.flag_generateNMIs = data & 0x80 >> 7
			ppu.t = (ppu.t & 0xF3FF) | ((uint16(data) & 0x03) << 10)
		}
	case 1:
		// PPUMASK
		ppu.flag_grayscale = data & 0x1 >> 0
		ppu.flag_showBackgroundLeft = data & 0x2 >> 1
		ppu.flag_showSpritesLeft = data & 0x4 >> 2
		ppu.flag_renderBackground = data & 0x8 >> 3
		ppu.flag_renderSprites = data & 0x10 >> 4
		ppu.flag_emphasizeRed = data & 0x20 >> 5
		ppu.flag_emphasizeGreen = data & 0x40 >> 6
		ppu.flag_emphasizeBlue = data & 0x80 >> 7
	case 3:
		// OAMADDR
		ppu.oamAddr = data
	case 4:
		// OAMDATA
		if !ppu.status_rendering {
			ppu.oam[ppu.oamAddr] = data
			ppu.oamAddr++
		}
	case 5:
		// PPUSCROLL
		// https://wiki.nesdev.com/w/index.php/PPU_scrolling#Register_controls
		if ppu.w == 0 {
			ppu.t = (ppu.t & 0xFFE0) | (uint16(data) >> 3)
			ppu.x = data & 0x7
			ppu.w = 1
		} else {
			ppu.t = (ppu.t & 0x8C1F) | ((uint16(data) & 0xF8) << 2) | ((uint16(data) & 0x7) << 12)
			ppu.w = 0
		}
	case 6:
		// PPUADDR
		if ppu.w == 0 {
			ppu.t = (ppu.t & 0x80FF) | ((uint16(data) & 0x3F) << 8)
			ppu.w = 1
		} else {
			ppu.t = (ppu.t & 0xFF00) | uint16(data)
			ppu.v = ppu.t
			ppu.w = 0
		}
		// fmt.Printf("PPUADDR : %.2X   %d  = (%.4X) %.4X (via %.4X)\n", data, ppu.w, ppu.t, ppu.v, nes.cpu.PC)
	case 7:
		// PPUDATA
		//fmt.Printf("PPUDATA : %.2X  %d\n", data, ppu.flag_incrementVram)
		ppu.mem.Write(address(ppu.v), data)
		if ppu.flag_incrementVram == 0 {
			ppu.v += 1
		} else {
			ppu.v += 32
		}
	case 0x4014:
		// OAMDMA
		nes.cpu.suspended = 513
		if nes.cpu.totalCycles%2 == 1 {
			nes.cpu.suspended += 1
		}

		addr := address(data) << 8
		for i := 0; i < 256; i++ {
			addr2 := addr + address(i)
			data := nes.cpu.mem.Read(addr2)
			ppu.oam[(ppu.oamAddr+byte(i))&0xFF] = data
		}
	}
}

func (ppu *Ppu) Emulate(cycles int) {
	cycles_left := cycles
	for cycles_left > 0 {
		ppu.cycles++
		ppu.tickCounter++
		if ppu.tickCounter == 341 || (ppu.tickCounter == 340 && ppu.scanlineCounter == -1 && ppu.frameCounter%2 == 1) {
			ppu.tickCounter = 0
			ppu.scanlineCounter++
			if ppu.scanlineCounter > 260 {
				ppu.scanlineCounter = -1
			}
		}

		if ppu.scanlineCounter == 241 && ppu.tickCounter == 1 {
			// VBLANK
			ppu.funcPushFrame()
			if ppu.flag_generateNMIs == 1 {
				ppu.nes.cpu.triggerInterruptNMI()
			}
			ppu.flag_vBlank = 1
			ppu.frameCounter += 1
			ppu.status_rendering = false
		}
		cycles_left--

		renderingEnabled := ppu.flag_renderBackground != 0 || ppu.flag_renderSprites != 0

		if ppu.scanlineCounter == -1 {
			if ppu.tickCounter == 1 {
				// prerender
				ppu.flag_sprite0Hit = 0
				ppu.flag_vBlank = 0
				ppu.flag_spriteOverflow = 0
				ppu.status_rendering = true
			}
			if ppu.tickCounter == 304 && renderingEnabled {
				// copy vertical scroll bits
				// v: IHGF.ED CBA..... = t: IHGF.ED CBA.....
				ppu.v = (ppu.v & 0x841F) | (ppu.t & 0x7BE0)
			}
		}

		// visible rendered scanlines
		if ppu.scanlineCounter >= 0 && ppu.scanlineCounter < 240 && renderingEnabled {
			/* ***** SPRITE EVALUATION ***** */
			if ppu.tickCounter >= 1 && ppu.tickCounter <= 64 {
				// https://wiki.nesdev.com/w/index.php/PPU_sprite_evaluation
				// Sprite Evaluation Stage 1: Clearing the Secondary OAM
				if ppu.tickCounter%2 == 0 {
					ppu.secondary_oam[(ppu.tickCounter-1)/2] = 0xFF
				}
			}
			if ppu.tickCounter == 65 {
				ppu.spriteEvaluationN = 0
				ppu.spriteEvaluationM = 0
				ppu.pendingNumScanlineSprites = 0
				ppu.spriteZeroAtNext = 0
			}
			if ppu.tickCounter >= 65 && ppu.tickCounter <= 256 {
				// Sprite Evaluation Stage 2: Loading the Secondary OAM
				spriteHeight := byte(8)
				if ppu.flag_spriteSize != 0 {
					spriteHeight = 16
				}

				if ppu.spriteEvaluationN < 64 && ppu.pendingNumScanlineSprites < 8 {
					if ppu.tickCounter%2 == 1 {
						// read from primary
						ppu.spriteEvaluationRead = ppu.oam[4*ppu.spriteEvaluationN+ppu.spriteEvaluationM]
					} else {
						// write to secondary
						ppu.secondary_oam[4*ppu.pendingNumScanlineSprites+ppu.spriteEvaluationM] = ppu.spriteEvaluationRead
						if ppu.spriteEvaluationM == 0 {
							// check to see if it's in range
							if byte(ppu.scanlineCounter) >= ppu.spriteEvaluationRead && byte(ppu.scanlineCounter) < ppu.spriteEvaluationRead+spriteHeight {
								// it's in range!
							} else {
								// not in range.
								ppu.spriteEvaluationM--
								ppu.spriteEvaluationN++
							}
						}
						if ppu.spriteEvaluationM == 3 {
							if ppu.spriteEvaluationN == 0 {
								ppu.spriteZeroAt = ppu.pendingNumScanlineSprites
							}
							ppu.spriteEvaluationN++
							ppu.spriteEvaluationM = 0
							ppu.pendingNumScanlineSprites += 1
						} else {
							ppu.spriteEvaluationM++
						}
					}
				}
			}
			if ppu.tickCounter >= 257 && ppu.tickCounter <= 320 {
				ppu.spriteEvaluationN = (ppu.tickCounter - 257) / 8
				ppu.numScanlineSprites = ppu.pendingNumScanlineSprites
				ppu.spriteZeroAt = ppu.spriteZeroAtNext
				if (ppu.tickCounter-257)%8 == 0 {
					// fetch x position, attribute into temporary latches and counters
					var ypos, tile, attribute, xpos byte
					if ppu.spriteEvaluationN < ppu.numScanlineSprites {
						ypos = ppu.secondary_oam[ppu.spriteEvaluationN*4+0]
						tile = ppu.secondary_oam[ppu.spriteEvaluationN*4+1]
						attribute = ppu.secondary_oam[ppu.spriteEvaluationN*4+2]
						xpos = ppu.secondary_oam[ppu.spriteEvaluationN*4+3]
					} else {
						ypos, tile, attribute, xpos = 0xFF, 0xFF, 0xFF, 0xFF
					}
					ppu.spriteXPositions[ppu.spriteEvaluationN], ppu.spriteAttributes[ppu.spriteEvaluationN] = int(xpos), attribute

					spriteTable := ppu.flag_spriteTableAddress
					tileRow := ppu.scanlineCounter - int(ypos)

					if ppu.flag_spriteSize != 0 {
						// 8x16 sprites
						spriteTable = tile & 0x1
						tile = tile & 0xFE
						if tileRow >= 8 {
							tile |= 1 - (attribute & 0x80 >> 7)
							tileRow += 8
						} else {
							tile |= attribute & 0x80 >> 7
						}
					}

					// fetch bitmap data into shift registers
					if attribute&0x80 > 0 {
						// flip sprite vertically
						tileRow = 7 - tileRow
					}
					var patternAddr address = 0
					patternAddr |= address(tileRow)
					patternAddr |= address(tile) << 4
					patternAddr |= address(spriteTable) << 12
					lo, hi := ppu.mem.Read(patternAddr), ppu.mem.Read(patternAddr+8)

					if attribute&0x40 > 0 {
						// flip sprite horizontally
						var hi2, lo2 byte
						for i := 0; i < 8; i++ {
							hi2 = (hi2 << 1) | (hi & 1)
							lo2 = (lo2 << 1) | (lo & 1)
							hi >>= 1
							lo >>= 1
						}
						lo, hi = lo2, hi2
					}

					ppu.spriteBitmapDataLo[ppu.spriteEvaluationN] = lo
					ppu.spriteBitmapDataHi[ppu.spriteEvaluationN] = hi
				}
			}
			/* ***** END SPRITE EVALUATION ***** */

			/* ***** DRAWING ! ***************** */
			if ppu.tickCounter >= 1 && ppu.tickCounter <= 256 {
				ppu.renderPixel()
			}

			// fetching tile data
			if ppu.scanlineCounter < 240 {
				if (ppu.tickCounter >= 1 && ppu.tickCounter <= 256) || (ppu.tickCounter >= 321 && ppu.tickCounter <= 336) {
					ppu.backgroundBitmapData <<= 4

					if ppu.tickCounter%8 == 0 {
						ppu.fetchTileData()
					}
				}
			}

			/* ***** UPDATE SCROLLING ********** */
			if ppu.tickCounter == 256 {
				ppu.incrementScrollY()
			}
			if ppu.tickCounter == 257 {
				// copy horizontal bits from t to v
				// v: ....F.. ...EDCBA = t: ....F.. ...EDCBA
				ppu.v = (ppu.v & 0xFBE0) | (ppu.t & 0x41F)
			}
			if ((ppu.tickCounter >= 321 && ppu.tickCounter <= 336) || (ppu.tickCounter >= 1 && ppu.tickCounter <= 256)) && (ppu.tickCounter%8 == 0) {
				ppu.incrementScrollX()
			}
		}
	}
}

func (ppu *Ppu) renderPixel() {
	x, y := ppu.tickCounter-1, ppu.scanlineCounter

	// background pixel
	backgroundPixel := byte(ppu.backgroundBitmapData >> (32 + ((7 - ppu.x) * 4)) & 0xF)

	// sprite pixel
	var spritePixel byte = 0
	var spriteIndex = 0
	for n := 0; n < ppu.numScanlineSprites; n++ {
		offset := x - int(ppu.spriteXPositions[n])
		if offset >= 0 && offset < 8 {
			attributes := ppu.spriteAttributes[n]
			data := ((ppu.spriteBitmapDataHi[n] & 0x80) >> 6) | ((ppu.spriteBitmapDataLo[n] & 0x80) >> 7)
			ppu.spriteBitmapDataHi[n] <<= 1
			ppu.spriteBitmapDataLo[n] <<= 1
			if data != 0 {
				spritePixel = 0x10 + data + 4*(attributes&0x3)
				spriteIndex = n
				break
			}
		}
	}

	// left screen hiding
	if x < 8 {
		if ppu.flag_showBackgroundLeft == 0 {
			backgroundPixel = 0
		}
		if ppu.flag_showSpritesLeft == 0 {
			spritePixel = 0
		}
	}

	var output byte = 0
	bgVisible, spVisible := backgroundPixel%4 != 0, spritePixel%4 != 0
	if !bgVisible && !spVisible {
		output = 0
	} else if !bgVisible {
		output = spritePixel
	} else if !spVisible {
		output = backgroundPixel
	} else {
		if spriteIndex == ppu.spriteZeroAt {
			ppu.flag_sprite0Hit = 1
		}

		spriteHasPriority := ppu.spriteAttributes[spriteIndex]&0x20 == 0
		if spriteHasPriority {
			output = spritePixel
		} else {
			output = backgroundPixel
		}
	}

	ppu.funcPushPixel(x, y, ppu.FetchColor(output))
}

func (ppu *Ppu) fetchTileData() {
	// run on (_ % 8 == 1) ticks in prerender and render scanlines
	// we need to fetch a tile AND the attribute data, combine them, and
	// shove them onto our queue of uh, stuff
	nametableAddress := 0x2000 | (ppu.v & 0x0FFF)
	nametableData := ppu.mem.Read(address(nametableAddress))
	attributeAddress := 0x23C0 | (ppu.v & 0x0C00) | ((ppu.v >> 4) & 0x38) | ((ppu.v >> 2) & 0x07)
	attributeData := ppu.mem.Read(address(attributeAddress))
	// process attribute data to select correct tile
	attributeData = ((attributeData >> (((ppu.v >> 4) & 4) | (ppu.v & 2))) & 3) << 2

	var patternAddr address = 0
	patternAddr |= address((ppu.v >> 12) & 0x7)
	patternAddr |= address(nametableData) << 4
	patternAddr |= address(ppu.flag_backgroundTableAddress) << 12
	patternLo, patternHi := ppu.mem.Read(patternAddr), ppu.mem.Read(patternAddr+8)

	var bitmap uint32 = 0
	for i := 0; i < 8; i++ {
		// shift on the data
		pixelData := attributeData | ((patternLo & 0x80) >> 7) | ((patternHi & 0x80) >> 6)
		patternLo <<= 1
		patternHi <<= 1
		bitmap = (bitmap << 4) | uint32(pixelData)
	}

	ppu.backgroundBitmapData |= uint64(bitmap)
}

func (ppu *Ppu) incrementScrollY() {
	if ppu.v&0x7000 != 0x7000 {
		ppu.v += 0x1000
	} else {
		ppu.v &= 0x8FFF
		y := (ppu.v & 0x03E0) >> 5
		if y == 29 {
			y = 0
			ppu.v ^= 0x0800
		} else if y == 31 {
			y = 0
		} else {
			y += 1
		}
		ppu.v = (ppu.v & 0xFC1F) | (y << 5)
	}
}

func (ppu *Ppu) incrementScrollX() {
	if ppu.v&0x001F == 31 {
		ppu.v &= 0xFFE0
		ppu.v ^= 0x0400
	} else {
		ppu.v += 1
	}
}

func (ppu *Ppu) FetchColor(index byte) color {
	return ppu.colors[ppu.palette[index&0x1F]]
}
