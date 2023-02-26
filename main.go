package main

import (
	"embed"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/text"
	logging "github.com/tsujio/game-logging-server/client"
	"github.com/tsujio/game-util/drawutil"
	"github.com/tsujio/game-util/loggingutil"
	"github.com/tsujio/game-util/resourceutil"
	"github.com/tsujio/game-util/touchutil"
)

const (
	gameName       = "four-color-theorem"
	screenWidth    = 640
	screenHeight   = 480
	maxTriangleNum = 30
)

//go:embed resources/*.ttf resources/*.dat resources/bgm-*.wav resources/*.png resources/secret
var resources embed.FS

var (
	fontL, fontM, fontS = resourceutil.ForceLoadFont(resources, "resources/PressStart2P-Regular.ttf", nil)
	audioContext        = audio.NewContext(48000)
	color0AudioData     = resourceutil.ForceLoadDecodedAudio(resources, "resources/color-0.wav.dat", audioContext)
	color1AudioData     = resourceutil.ForceLoadDecodedAudio(resources, "resources/color-1.wav.dat", audioContext)
	color2AudioData     = resourceutil.ForceLoadDecodedAudio(resources, "resources/color-2.wav.dat", audioContext)
	color3AudioData     = resourceutil.ForceLoadDecodedAudio(resources, "resources/color-3.wav.dat", audioContext)
	gameStartAudioData  = resourceutil.ForceLoadDecodedAudio(resources, "resources/魔王魂 効果音 システム49.mp3.dat", audioContext)
	playStartAudioData  = resourceutil.ForceLoadDecodedAudio(resources, "resources/魔王魂 効果音 笛01.mp3.dat", audioContext)
	completeAudioData   = resourceutil.ForceLoadDecodedAudio(resources, "resources/魔王魂 効果音 物音15.mp3.dat", audioContext)
	bgmPlayer           = resourceutil.ForceCreateBGMPlayer(resources, "resources/bgm-four-color-theorem.wav", audioContext)
	skyImg              = loadImage("resources/sky.png")
	surfaceImg          = loadImage("resources/surface.png")
)

func loadImage(path string) *ebiten.Image {
	f, err := resources.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		log.Fatal(err)
	}
	return ebiten.NewImageFromImage(img)
}

type Point struct {
	x, y float64
}

func (p *Point) norm() float64 {
	return math.Sqrt(math.Pow(p.x, 2) + math.Pow(p.y, 2))
}

func (p *Point) add(q *Point) *Point {
	return &Point{x: p.x + q.x, y: p.y + q.y}
}

func (p *Point) sub(q *Point) *Point {
	return &Point{x: p.x - q.x, y: p.y - q.y}
}

func (p *Point) mul(a float64) *Point {
	return &Point{x: p.x * a, y: p.y * a}
}

func (p *Point) div(a float64) *Point {
	return &Point{x: p.x / a, y: p.y / a}
}

func (p *Point) innerProd(q *Point) float64 {
	return p.x*q.x + p.y*q.y
}

func (p *Point) outerProdZ(q *Point) float64 {
	return p.x*q.y - p.y*q.x
}

func (p *Point) rotate(theta float64) *Point {
	return &Point{x: math.Cos(theta)*p.x - math.Sin(theta)*p.y, y: math.Sin(theta)*p.x + math.Cos(theta)*p.y}
}

type Line [2]Point

func (l *Line) equals(m *Line) bool {
	return l[0] == m[0] && l[1] == m[1] || l[0] == m[1] && l[1] == m[0]
}

func (l *Line) cross(m *Line) bool {
	z := l[1].sub(&l[0]).outerProdZ(m[1].sub(&m[0]))
	if math.Abs(z) < 1e-3 {
		return false
	}

	v := m[0].sub(&l[0])
	z1 := v.outerProdZ(l[1].sub(&l[0]))
	z2 := v.outerProdZ(m[1].sub(&m[0]))
	t1 := z2 / z
	t2 := z1 / z

	return 0 <= t1 && t1 <= 1 && 0 <= t2 && t2 <= 1
}

func (l *Line) normSq() float64 {
	return math.Pow(l[0].x-l[1].x, 2) + math.Pow(l[0].y-l[1].y, 2)
}

func (l *Line) distanceSq(p *Point) float64 {
	return math.Pow((l[1].x-l[0].x)*(l[0].y-p.y)-(l[1].y-l[0].y)*(l[0].x-p.x), 2) / l.normSq()
}

type Triangle [3]Point

func (t *Triangle) equals(s *Triangle) bool {
	return t[0] == s[0] && t[1] == s[1] && t[2] == s[2] ||
		t[0] == s[0] && t[1] == s[2] && t[2] == s[1] ||
		t[0] == s[1] && t[1] == s[0] && t[2] == s[2] ||
		t[0] == s[1] && t[1] == s[2] && t[2] == s[0] ||
		t[0] == s[2] && t[1] == s[1] && t[2] == s[0] ||
		t[0] == s[2] && t[1] == s[0] && t[2] == s[1]
}

func (t *Triangle) covers(p *Point) bool {
	v0 := t[0].sub(&t[1])
	v1 := t[1].sub(&t[2])
	v2 := t[2].sub(&t[0])

	vp0 := p.sub(&t[0])
	vp1 := p.sub(&t[1])
	vp2 := p.sub(&t[2])

	z0 := v0.outerProdZ(vp0)
	z1 := v1.outerProdZ(vp1)
	z2 := v2.outerProdZ(vp2)

	return z0 > 0 && z1 > 0 && z2 > 0 || z0 < 0 && z1 < 0 && z2 < 0
}

func (t *Triangle) contains(l *Line) bool {
	return (l[0] == t[0] || l[0] == t[1] || l[0] == t[2]) &&
		(l[1] == t[0] || l[1] == t[1] || l[1] == t[2])
}

func (t *Triangle) shareLineWith(s *Triangle) bool {
	for i := 0; i < 3; i++ {
		l := Line([2]Point{s[i%3], s[(i+1)%3]})
		if t.contains(&l) {
			return true
		}
	}
	return false
}

func (t *Triangle) collidesWith(s *Triangle) bool {
	ts := []*Triangle{t, s, t}

	for tsi := 0; tsi < 2; tsi++ {
		t1 := ts[tsi]
		t2 := ts[tsi+1]

		for i := 0; i < 3; i++ {
			v := t1[(i+1)%3].sub(&t1[i%3])
			sep := &Point{x: v.y, y: -v.x}
			sep = sep.div(sep.norm())

			t1p1 := sep.innerProd(&t1[i%3])
			t1p2 := sep.innerProd(&t1[(i+2)%3])
			t1pMin := math.Min(t1p1, t1p2)
			t1pMax := math.Max(t1p1, t1p2)

			t2p1 := sep.innerProd(&t2[0])
			t2p2 := sep.innerProd(&t2[1])
			t2p3 := sep.innerProd(&t2[2])
			t2pMin := math.Min(t2p1, t2p2)
			t2pMin = math.Min(t2pMin, t2p3)
			t2pMax := math.Max(t2p1, t2p2)
			t2pMax = math.Max(t2pMax, t2p3)

			if t2pMin <= t1pMin && t1pMin < t2pMax ||
				t2pMin < t1pMax && t1pMax <= t2pMax ||
				t1pMin <= t2pMin && t2pMin < t1pMax ||
				t1pMin < t2pMax && t2pMax <= t1pMax {
				continue
			}

			return false
		}
	}

	return true
}

type AreaStatus int

const (
	AreaStatusInitial = iota
	AreaStatusOK
	AreaStatusNG
)

type Area struct {
	Triangle
	color     int
	adjacents []*Area
	status    AreaStatus
}

var vertexImg = drawutil.CreatePatternImage([][]rune{
	[]rune("  #  "),
	[]rune("  #  "),
	[]rune(" ### "),
	[]rune("#####"),
	[]rune(" ### "),
	[]rune("  #  "),
	[]rune("  #  "),
}, &drawutil.CreatePatternImageOption[rune]{
	Color:   color.RGBA{0xf5, 0xdb, 0x49, 0xff},
	DotSize: 1.5,
})

func (a *Area) DrawVertices(screen *ebiten.Image, brightness float64) {
	for _, p := range a.Triangle {
		opts := &ebiten.DrawImageOptions{}
		opts.ColorM.Scale(1.0, 1.0, 1.0, brightness)
		drawutil.DrawImageAt(screen, vertexImg, p.x, p.y, opts)
	}
}

var emptyImage = func() *ebiten.Image {
	img := ebiten.NewImage(3, 3)
	img.Fill(color.White)
	return img
}()

func (a *Area) getColorScales() (r, g, b, alpha float32) {
	switch a.color {
	case -1:
		alpha = 0.0
	case 0:
		r = 1.0
		alpha = 0.3
	case 1:
		g = 1.0
		alpha = 0.3
	case 2:
		b = 1.0
		alpha = 0.3
	case 3:
		r = 1.0
		g = 1.0
		alpha = 0.3
	}
	return
}

func (a *Area) Draw(screen *ebiten.Image) {
	var vertices []ebiten.Vertex
	for i := 0; i < 3; i++ {
		v := ebiten.Vertex{
			DstX: float32(a.Triangle[i].x),
			DstY: float32(a.Triangle[i].y),
			SrcX: 0,
			SrcY: 0,
		}
		v.ColorR, v.ColorG, v.ColorB, v.ColorA = a.getColorScales()
		vertices = append(vertices, v)
	}
	indices := []uint16{0, 1, 2}
	op := &ebiten.DrawTrianglesOptions{}
	screen.DrawTriangles(vertices, indices, emptyImage.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image), op)
}

type TriangleEffect struct {
	Triangle
	ticks                  uint64
	colorR, colorG, colorB float32
}

func (e *TriangleEffect) Update() {
	e.ticks++
}

func (e *TriangleEffect) Draw(screen *ebiten.Image) {
	scale := 1.1 + float64(e.ticks)/60
	alpha := 0.2 * (1.0 - float32(e.ticks)/60)

	center := e.Triangle[0].add(&e.Triangle[1]).add(&e.Triangle[2]).div(3)

	var vertices []ebiten.Vertex
	for i := 0; i < 3; i++ {
		p := e.Triangle[i].sub(center).mul(scale).add(center)
		v := ebiten.Vertex{
			DstX: float32(p.x),
			DstY: float32(p.y),
			SrcX: 0,
			SrcY: 0,
		}
		v.ColorR, v.ColorG, v.ColorB, v.ColorA = e.colorR, e.colorG, e.colorB, alpha
		vertices = append(vertices, v)
	}
	indices := []uint16{0, 1, 2}
	op := &ebiten.DrawTrianglesOptions{}
	screen.DrawTriangles(vertices, indices, emptyImage.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image), op)
}

type Star struct {
	Point
	r float64
}

func (s *Star) Draw(screen *ebiten.Image, brightness float64) {
	ebitenutil.DrawCircle(screen, s.x, s.y, s.r, color.RGBA{0xff, 0xff, 0xff, uint8(0xff * brightness)})
}

type ShootingStar struct {
	Point
	r      float64
	vx, vy float64
	ticks  uint64
}

func (s *ShootingStar) Update() {
	s.ticks++
	s.x += s.vx
	s.y += s.vy
}

func (s *ShootingStar) Draw(screen *ebiten.Image) {
	ts := math.Min(float64(s.ticks), 30)
	te := math.Max(float64(s.ticks)-30, 0)
	ebitenutil.DrawLine(screen, s.x-s.vx*ts, s.y-s.vy*ts, s.x-s.vx*te, s.y-s.vy*te, color.RGBA{0xff, 0xff, 0xff, 0x30})

	if s.ticks < 30 {
		ebitenutil.DrawCircle(screen, s.x, s.y, s.r, color.RGBA{0xff, 0xff, 0xff, 0x7a})
	}
}

type GameMode int

const (
	GameModeTitle GameMode = iota
	GameModeOpening
	GameModePlaying
	GameModeGameOver
	GameModeRanking
)

type Game struct {
	playerID             string
	playID               string
	fixedRandomSeed      int64
	touchContext         *touchutil.TouchContext
	random               *rand.Rand
	mode                 GameMode
	ticksFromModeStart   uint64
	score                int
	rankingCh            <-chan []logging.GameScore
	ranking              []logging.GameScore
	stars                []Star
	shootingStars        []ShootingStar
	areas                []Area
	triangleEffects      []TriangleEffect
	openingLineDrawOrder [][]Line
}

func (g *Game) Update() error {
	g.touchContext.Update()

	g.ticksFromModeStart++

	loggingutil.SendTouchLog(gameName, g.playerID, g.playID, g.ticksFromModeStart, g.touchContext)

	switch g.mode {
	case GameModeTitle:
		if g.touchContext.IsJustTouched() {
			g.setNextMode(GameModeOpening)

			loggingutil.SendLog(gameName, g.playerID, g.playID, map[string]interface{}{
				"action": "start_game",
			})

			audio.NewPlayerFromBytes(audioContext, gameStartAudioData).Play()
		}
	case GameModeOpening:
		if g.random.Int()%120 == 0 {
			g.shootingStars = append(g.shootingStars, ShootingStar{
				Point: Point{
					x: screenWidth * g.random.Float64(),
					y: screenHeight * g.random.Float64(),
				},
				r:  2.0,
				vx: -3.0,
				vy: 3.0,
			})
		}

		var newShootingStars []ShootingStar
		for i := range g.shootingStars {
			s := &g.shootingStars[i]
			s.Update()

			if s.ticks < 60 {
				newShootingStars = append(newShootingStars, *s)
			}
		}
		g.shootingStars = newShootingStars

		if g.ticksFromModeStart > 10*60 {
			g.setNextMode(GameModePlaying)

			audio.NewPlayerFromBytes(audioContext, playStartAudioData).Play()
		}
	case GameModePlaying:
		if g.ticksFromModeStart%600 == 0 {
			loggingutil.SendLog(gameName, g.playerID, g.playID, map[string]interface{}{
				"action": "playing",
				"ticks":  g.ticksFromModeStart,
				"score":  g.score,
			})
		}

		if g.ticksFromModeStart == 60 {
			bgmPlayer.Rewind()
			bgmPlayer.Play()
		}

		g.score = int(g.ticksFromModeStart)

		if g.touchContext.IsJustTouched() {
			pos := g.touchContext.GetTouchPosition()
			for i := range g.areas {
				a := &g.areas[i]
				if a.Triangle.covers(&Point{x: float64(pos.X), y: float64(pos.Y)}) {
					a.color = (a.color + 1) % 4

					cr, cg, cb, _ := a.getColorScales()
					e := TriangleEffect{
						Triangle: a.Triangle,
						colorR:   cr,
						colorG:   cg,
						colorB:   cb,
					}
					g.triangleEffects = append(g.triangleEffects, e)

					switch a.color {
					case 0:
						audio.NewPlayerFromBytes(audioContext, color0AudioData).Play()
					case 1:
						audio.NewPlayerFromBytes(audioContext, color1AudioData).Play()
					case 2:
						audio.NewPlayerFromBytes(audioContext, color2AudioData).Play()
					case 3:
						audio.NewPlayerFromBytes(audioContext, color3AudioData).Play()
					}

					break
				}
			}
		}

		if g.random.Int()%120 == 0 {
			g.shootingStars = append(g.shootingStars, ShootingStar{
				Point: Point{
					x: screenWidth * g.random.Float64(),
					y: screenHeight * g.random.Float64(),
				},
				r:  2.0,
				vx: -3.0,
				vy: 3.0,
			})
		}

		var newTriangleEffects []TriangleEffect
		for i := range g.triangleEffects {
			e := &g.triangleEffects[i]
			e.Update()

			if e.ticks < 10 {
				newTriangleEffects = append(newTriangleEffects, *e)
			}
		}
		g.triangleEffects = newTriangleEffects

		var newShootingStars []ShootingStar
		for i := range g.shootingStars {
			s := &g.shootingStars[i]
			s.Update()

			if s.ticks < 60 {
				newShootingStars = append(newShootingStars, *s)
			}
		}
		g.shootingStars = newShootingStars

		allOK := true
		for i := range g.areas {
			a := &g.areas[i]
			a.status = AreaStatusInitial
			if a.color != -1 {
				a.status = AreaStatusOK
				for _, ad := range a.adjacents {
					if a.color == ad.color {
						a.status = AreaStatusNG
						break
					}
				}
			}
			if a.status != AreaStatusOK {
				allOK = false
			}
		}

		if allOK {
			loggingutil.SendLog(gameName, g.playerID, g.playID, map[string]interface{}{
				"action": "game_over",
				"score":  g.score,
			})

			g.triangleEffects = nil
			for _, a := range g.areas {
				cr, cg, cb, _ := a.getColorScales()
				e := TriangleEffect{
					Triangle: a.Triangle,
					colorR:   cr,
					colorG:   cg,
					colorB:   cb,
				}
				g.triangleEffects = append(g.triangleEffects, e)
			}

			g.setNextMode(GameModeGameOver)

			g.rankingCh = loggingutil.RegisterScoreToRankingAsync(gameName, g.playerID, g.playID, g.score)

			audio.NewPlayerFromBytes(audioContext, completeAudioData).Play()
		}
	case GameModeGameOver:
		if g.random.Int()%30 == 0 {
			g.shootingStars = append(g.shootingStars, ShootingStar{
				Point: Point{
					x: screenWidth * g.random.Float64(),
					y: screenHeight * g.random.Float64(),
				},
				r:  2.0,
				vx: -3.0,
				vy: 3.0,
			})
		}

		var newTriangleEffects []TriangleEffect
		for i := range g.triangleEffects {
			e := &g.triangleEffects[i]
			e.Update()

			if e.ticks < 60 {
				newTriangleEffects = append(newTriangleEffects, *e)
			}
		}
		g.triangleEffects = newTriangleEffects

		var newShootingStars []ShootingStar
		for i := range g.shootingStars {
			s := &g.shootingStars[i]
			s.Update()

			if s.ticks < 60 {
				newShootingStars = append(newShootingStars, *s)
			}
		}
		g.shootingStars = newShootingStars

		if g.ticksFromModeStart > 60 && g.touchContext.IsJustTouched() {
			g.initialize()
			bgmPlayer.Pause()
		}
	}

	return nil
}

func (g *Game) drawSky(screen *ebiten.Image) {
	opts := &ebiten.DrawImageOptions{}
	screen.DrawImage(skyImg, opts)
}

func (g *Game) drawSurface(screen *ebiten.Image) {
	opts := &ebiten.DrawImageOptions{}
	screen.DrawImage(surfaceImg, opts)
}

func (g *Game) drawTitle(screen *ebiten.Image) {
	text.Draw(screen, "FOUR", fontL.Face, 25, 75, color.White)
	text.Draw(screen, "COLOR", fontL.Face, 185, 75, color.White)
	text.Draw(screen, "THEOREM", fontL.Face, 370, 75, color.White)

	usageTexts := []string{"[TAP] Change color"}
	for i, s := range usageTexts {
		text.Draw(screen, s, fontS.Face, screenWidth/2-len(s)*int(fontS.FaceOptions.Size)/2, 350+i*int(fontS.FaceOptions.Size*1.8), color.White)
	}

	creditTexts := []string{"CREATOR: NAOKI TSUJIO", "FONT: Press Start 2P by CodeMan38", "SOUND EFFECT: MaouDamashii"}
	for i, s := range creditTexts {
		text.Draw(screen, s, fontS.Face, screenWidth/2-len(s)*int(fontS.FaceOptions.Size)/2, 420+i*int(fontS.FaceOptions.Size*1.8), color.White)
	}
}

func (g *Game) drawProgress(screen *ebiten.Image) {
	progress := 0.0
	for _, a := range g.areas {
		if a.color != -1 {
			ng := false
			for _, ad := range a.adjacents {
				if a.color == ad.color {
					ng = true
					break
				}
			}
			if !ng {
				progress += 1.0
			}
		}
	}
	progress /= float64(len(g.areas))

	t := fmt.Sprintf("%d%%", int(math.Floor(progress*100)))
	text.Draw(screen, t, fontS.Face, screenWidth-len(t)*int(fontS.FaceOptions.Size)-10, 20, color.White)
}

func (g *Game) drawOpening(screen *ebiten.Image) {
	if g.ticksFromModeStart < 60 {
		g.drawSurface(screen)
	} else if g.ticksFromModeStart < 360 {
		ticks := g.ticksFromModeStart - 60

		starBrightness := math.Min(float64(ticks)/150, 1.0)

		for _, s := range g.stars {
			s.Draw(screen, starBrightness)
		}

		for _, s := range g.shootingStars {
			s.Draw(screen)
		}

		vertexBrightness := math.Max((float64(ticks)-150)/150, 0.0)
		for _, a := range g.areas {
			a.DrawVertices(screen, vertexBrightness)
		}

		g.drawSurface(screen)
	} else {
		ticks := int(g.ticksFromModeStart - 360)

		for _, s := range g.stars {
			s.Draw(screen, 1.0)
		}

		for _, s := range g.shootingStars {
			s.Draw(screen)
		}

		for _, a := range g.areas {
			a.DrawVertices(screen, 1.0)
		}

		ticksPerIndex := (600 - 360) / len(g.openingLineDrawOrder)

		index := int(ticks / ticksPerIndex)
		if index > len(g.openingLineDrawOrder) {
			index = len(g.openingLineDrawOrder)
		}
		for i := 0; i < index; i++ {
			for _, l := range g.openingLineDrawOrder[i] {
				ebitenutil.DrawLine(screen, l[0].x, l[0].y, l[1].x, l[1].y, color.White)
			}
		}
		if index < len(g.openingLineDrawOrder) {
			for _, l := range g.openingLineDrawOrder[index] {
				v := l[1].sub(&l[0])
				v = v.mul(float64(ticks%ticksPerIndex) / float64(ticksPerIndex))
				p := l[0].add(v)
				ebitenutil.DrawLine(screen, l[0].x, l[0].y, p.x, p.y, color.White)
			}
		}

		g.drawSurface(screen)
	}
}

func (g *Game) drawScore(screen *ebiten.Image) {
	secs := g.score / 60
	s := fmt.Sprintf("%d:%02d", secs/60, secs%60)
	text.Draw(screen, s, fontS.Face, screenWidth/2-len(s)*int(fontS.FaceOptions.Size)/2, 20, color.White)
}

func (g *Game) drawGameOver(screen *ebiten.Image) {
	var s string

	s = "Complete!"
	text.Draw(screen, s, fontS.Face, screenWidth/2-len(s)*int(fontS.FaceOptions.Size)/2, 400, color.White)

	secs := g.score / 60
	s = fmt.Sprintf("Your time is %d:%02d", secs/60, secs%60)
	text.Draw(screen, s, fontS.Face, screenWidth/2-len(s)*int(fontS.FaceOptions.Size)/2, 420, color.White)
}

func (g *Game) Draw(screen *ebiten.Image) {
	g.drawSky(screen)

	switch g.mode {
	case GameModeTitle:
		for _, s := range g.stars {
			s.Draw(screen, 1.0)
		}

		var areas []Area
		c := Point{x: screenWidth / 2, y: 200}
		r := 100.0
		for i := 0; i < 6; i++ {
			theta1 := float64(i)*2*math.Pi/6 + math.Pi/2
			theta2 := float64((i+1)%6)*2*math.Pi/6 + math.Pi/2
			p1 := c.add(&Point{
				x: r * math.Cos(theta1),
				y: r * math.Sin(theta1),
			})
			p2 := c.add(&Point{
				x: r * math.Cos(theta2),
				y: r * math.Sin(theta2),
			})
			areas = append(areas, Area{
				Triangle: Triangle([3]Point{c, *p1, *p2}),
				color:    i % 4,
			})
		}
		areas = append(areas, Area{
			Triangle: Triangle([3]Point{
				areas[1].Triangle[1],
				areas[1].Triangle[2],
				{
					x: c.x - r*2*math.Sin(math.Pi/3),
					y: c.y,
				},
			}),
			color: 3,
		})
		areas = append(areas, Area{
			Triangle: Triangle([3]Point{
				areas[4].Triangle[1],
				areas[4].Triangle[2],
				{
					x: c.x + r*2*math.Sin(math.Pi/3),
					y: c.y,
				},
			}),
			color: 2,
		})
		for _, a := range areas {
			a.Draw(screen)
			a.DrawVertices(screen, 1.0)
		}

		for _, lines := range g.getLinesWithDrawOrder(areas) {
			for _, line := range lines {
				ebitenutil.DrawLine(screen, line[0].x, line[0].y, line[1].x, line[1].y, color.White)
			}
		}

		g.drawSurface(screen)

		g.drawTitle(screen)
	case GameModeOpening:
		g.drawOpening(screen)
	case GameModePlaying:
		for _, s := range g.stars {
			s.Draw(screen, 1.0)
		}

		for _, s := range g.shootingStars {
			s.Draw(screen)
		}

		for _, a := range g.areas {
			a.Draw(screen)
			a.DrawVertices(screen, 1.0)
		}

		for _, lines := range g.openingLineDrawOrder {
			for _, line := range lines {
				ebitenutil.DrawLine(screen, line[0].x, line[0].y, line[1].x, line[1].y, color.White)
			}
		}

		for _, e := range g.triangleEffects {
			e.Draw(screen)
		}

		g.drawSurface(screen)

		g.drawProgress(screen)

		g.drawScore(screen)
	default:
		for _, s := range g.stars {
			s.Draw(screen, 1.0)
		}

		for _, s := range g.shootingStars {
			s.Draw(screen)
		}

		for _, a := range g.areas {
			a.Draw(screen)
			a.DrawVertices(screen, 1.0)
		}

		for _, lines := range g.openingLineDrawOrder {
			for _, line := range lines {
				ebitenutil.DrawLine(screen, line[0].x, line[0].y, line[1].x, line[1].y, color.White)
			}
		}

		for _, e := range g.triangleEffects {
			e.Draw(screen)
		}

		g.drawSurface(screen)

		g.drawProgress(screen)

		g.drawScore(screen)

		g.drawGameOver(screen)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func (g *Game) setNextMode(mode GameMode) {
	g.mode = mode
	g.ticksFromModeStart = 0
}

func (g *Game) generateTriangles(seed *Triangle) []Triangle {
	findLinesToExtend := func(triangles []Triangle) (lines []struct {
		line *Line
		pair *Triangle
	}) {
		for _, t := range triangles {
			for i := 0; i < 3; i++ {
				l := Line([2]Point{t[i%3], t[(i+1)%3]})

				// Find pairs (a pair is a triangle that contains the same line)
				trs := make([]Triangle, 0)
				for _, tr := range triangles {
					if tr.contains(&l) {
						trs = append(trs, tr)
					}
				}

				// A line can be contained by at most two triangles
				if len(trs) > 1 {
					continue
				}

				found := false
				for _, item := range lines {
					if item.line.equals(&l) {
						found = true
						break
					}
				}
				if found {
					continue
				}

				lines = append(lines, struct {
					line *Line
					pair *Triangle
				}{line: &l, pair: &trs[0]})
			}
		}

		center := Point{x: screenWidth / 2, y: screenHeight / 2}
		sort.Slice(lines, func(i, j int) bool {
			return lines[i].line.distanceSq(&center) < lines[j].line.distanceSq(&center)
		})

		return
	}

	extendLine := func(triangles []Triangle, line *Line, pair *Triangle) *Triangle {
		v := line[1].sub(&line[0])

		// Find the third point of the pair
		var p Point
		if (line[0] == pair[0] || line[0] == pair[1]) && (line[1] == pair[0] || line[1] == pair[1]) {
			p = pair[2]
		} else if (line[0] == pair[1] || line[0] == pair[2]) && (line[1] == pair[1] || line[1] == pair[2]) {
			p = pair[0]
		} else {
			p = pair[1]
		}

		// Determine new point at the opposite side of the pair
		theta := math.Pi/3 + math.Pi/4*g.random.NormFloat64()
		if v.outerProdZ(p.sub(&line[0])) > 0 {
			theta *= -1
		}
		newPoint := line[0].add(v.rotate(theta).div(v.norm()).mul(100.0))

		// Ensure the new point is within the screen
		newPoint.x = math.Max(newPoint.x, 5)
		newPoint.x = math.Min(newPoint.x, screenWidth-5)
		newPoint.y = math.Max(newPoint.y, 5)
		newPoint.y = math.Min(newPoint.y, screenHeight-120)

		triangle := Triangle{
			line[0],
			line[1],
			*newPoint,
		}

		// Ensure the new triangle does not collide with existing ones
		collide := false
		for _, t := range triangles {
			if t.equals(&triangle) || t.collidesWith(&triangle) {
				collide = true
				break
			}
		}
		if collide {
			return nil
		}

		// Ensure the new triangle has sufficient angles
		for i := 0; i < 3; i++ {
			v1 := triangle[(i+1)%3].sub(&triangle[i%3])
			v2 := triangle[(i+2)%3].sub(&triangle[i%3])
			cos := v1.innerProd(v2) / v1.norm() / v2.norm()
			if cos > math.Cos(math.Pi/6) {
				return nil
			}
		}

		return &triangle
	}

	getNewTrianglesFromExistingPoints := func(triangles []Triangle) []Triangle {
		newTriangles := make([]Triangle, 0)
		for _, t1 := range triangles {
			for i := 0; i < 3; i++ {
				l1 := Line([2]Point{t1[i%3], t1[(i+1)%3]})
				for _, t2 := range triangles {
					for j := 0; j < 3; j++ {
						l2 := Line([2]Point{t2[j%3], t2[(j+1)%3]})

						if l1.equals(&l2) {
							continue
						}

						// Make triangle from l1 and l2
						var newTriangle Triangle
						if l1[0] == l2[0] {
							v1 := l1[1].sub(&l1[0])
							v2 := l2[1].sub(&l2[0])
							cos := v1.innerProd(v2) / v1.norm() / v2.norm()
							if cos < 0 {
								continue
							}
							newTriangle = [3]Point{l1[0], l1[1], l2[1]}
						} else if l1[0] == l2[1] {
							v1 := l1[1].sub(&l1[0])
							v2 := l2[0].sub(&l2[1])
							cos := v1.innerProd(v2) / v1.norm() / v2.norm()
							if cos < 0 {
								continue
							}
							newTriangle = [3]Point{l1[0], l1[1], l2[0]}
						} else if l1[1] == l2[0] {
							v1 := l1[0].sub(&l1[1])
							v2 := l2[1].sub(&l2[0])
							cos := v1.innerProd(v2) / v1.norm() / v2.norm()
							if cos < 0 {
								continue
							}
							newTriangle = [3]Point{l1[1], l1[0], l2[1]}
						} else {
							continue
						}

						// Ensure the new triangle does not collide with existing ones
						collide := false
						var trs []Triangle
						trs = append(trs, triangles...)
						trs = append(trs, newTriangles...)
						for _, t := range trs {
							if t.equals(&newTriangle) || t.collidesWith(&newTriangle) {
								collide = true
								break
							}
						}
						if collide {
							continue
						}

						// Ensure the new triangle has sufficient angles
						for k := 0; k < 3; k++ {
							v1 := newTriangle[(k+1)%3].sub(&newTriangle[k%3])
							v2 := newTriangle[(k+2)%3].sub(&newTriangle[k%3])
							cos := v1.innerProd(v2) / v1.norm() / v2.norm()
							if cos > math.Cos(math.Pi/6) {
								return nil
							}
						}

						newTriangles = append(newTriangles, newTriangle)
					}
				}
			}
		}

		return newTriangles
	}

	triangles := []Triangle{*seed}
	for {
		if len(triangles) > maxTriangleNum {
			break
		}

		lines := findLinesToExtend(triangles)
		if len(lines) == 0 {
			break
		}

		found := false
		for _, line := range lines {
			for retry := 0; retry < 3; retry++ {
				if t := extendLine(triangles, line.line, line.pair); t != nil {
					found = true
					triangles = append(triangles, *t)
					break
				}
			}
			if found {
				break
			}
		}

		if !found {
			if len(triangles) < 10 {
				continue
			}

			break
		}

		if newTriangles := getNewTrianglesFromExistingPoints(triangles); newTriangles == nil {
			triangles = triangles[:1]
			continue
		} else {
			triangles = append(triangles, newTriangles...)
		}
	}

	return triangles
}

func (g *Game) getLinesWithDrawOrder(areas []Area) [][]Line {
	var linesList [][]Line

	t0 := areas[0].Triangle
	linesList = append(linesList, []Line{
		Line([2]Point{t0[0], t0[1]}),
		Line([2]Point{t0[1], t0[2]}),
		Line([2]Point{t0[2], t0[0]}),
	})

	lineExists := func(line *Line, linesList [][]Line, newLines []Line) bool {
		for _, ls := range linesList {
			for _, l := range ls {
				if l.equals(line) {
					return true
				}
			}
		}
		for _, l := range newLines {
			if l.equals(line) {
				return true
			}
		}
		return false
	}

	for {
		lines := linesList[len(linesList)-1]
		var newLines []Line
		for _, a := range areas {
			for i := 0; i < 3; i++ {
				for _, line := range lines {
					if line[1] == a.Triangle[i] {
						l1 := Line([2]Point{a.Triangle[i], a.Triangle[(i+1)%3]})
						l2 := Line([2]Point{a.Triangle[i], a.Triangle[(i+2)%3]})
						if !lineExists(&l1, linesList, newLines) {
							newLines = append(newLines, l1)
						}
						if !lineExists(&l2, linesList, newLines) {
							newLines = append(newLines, l2)
						}
					}
				}
			}
		}
		if len(newLines) == 0 {
			break
		}
		linesList = append(linesList, newLines)
	}

	return linesList
}

func (g *Game) initialize() {
	var playID string
	if playIDObj, err := uuid.NewRandom(); err == nil {
		playID = playIDObj.String()
	}
	g.playID = playID

	var seed int64
	if g.fixedRandomSeed != 0 {
		seed = g.fixedRandomSeed
	} else {
		seed = time.Now().Unix()
	}

	loggingutil.SendLog(gameName, g.playerID, g.playID, map[string]interface{}{
		"action": "initialize",
		"seed":   seed,
	})

	g.random = rand.New(rand.NewSource(seed))
	g.score = 0
	g.rankingCh = nil
	g.ranking = nil
	g.stars = nil
	g.shootingStars = nil
	g.areas = nil
	g.triangleEffects = nil
	g.openingLineDrawOrder = nil

	for i := 0; i < 500; i++ {
		g.stars = append(g.stars, Star{
			Point: Point{
				x: screenWidth * g.random.Float64(),
				y: (screenHeight - 50) * g.random.Float64(),
			},
			r: math.Max(1.0+0.5*g.random.NormFloat64(), 0.5),
		})
	}

	t0 := Triangle([3]Point{
		{x: 1 * screenWidth / 2, y: 2 * screenHeight / 5},
		{x: 2 * screenWidth / 5, y: 3 * screenHeight / 5},
		{x: 3 * screenWidth / 5, y: 3 * screenHeight / 5},
	})
	triangles := g.generateTriangles(&t0)
	for _, t := range triangles {
		g.areas = append(g.areas, Area{
			Triangle: t,
			color:    -1,
			status:   AreaStatusInitial,
		})
	}
	for i := range g.areas {
		a := &g.areas[i]
		for j := range g.areas {
			b := &g.areas[j]
			if a.Triangle.equals(&b.Triangle) {
				continue
			}
			if a.Triangle.shareLineWith(&b.Triangle) {
				a.adjacents = append(a.adjacents, b)
			}
		}
	}

	g.openingLineDrawOrder = g.getLinesWithDrawOrder(g.areas)

	g.setNextMode(GameModeTitle)
}

func main() {
	if os.Getenv("GAME_LOGGING") == "1" {
		secret, err := resources.ReadFile("resources/secret")
		if err == nil {
			logging.Enable(string(secret))
		}
	} else {
		logging.Disable()
	}

	var randomSeed int64
	if seed, err := strconv.Atoi(os.Getenv("GAME_RAND_SEED")); err == nil {
		randomSeed = int64(seed)
	}

	playerID := os.Getenv("GAME_PLAYER_ID")
	if playerID == "" {
		if playerIDObj, err := uuid.NewRandom(); err == nil {
			playerID = playerIDObj.String()
		}
	}

	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Four Color Theorem")

	game := &Game{
		playerID:        playerID,
		fixedRandomSeed: randomSeed,
		touchContext:    touchutil.CreateTouchContext(),
	}
	game.initialize()

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}
