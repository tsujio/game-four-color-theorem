package main

import (
	"embed"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	logging "github.com/tsujio/game-logging-server/client"
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

//go:embed resources/*.ttf resources/*.dat resources/bgm-*.wav resources/secret
var resources embed.FS

var (
	fontL, fontM, fontS = resourceutil.ForceLoadFont(resources, "resources/PressStart2P-Regular.ttf", nil)
	audioContext        = audio.NewContext(48000)
)

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

var emptyImage = func() *ebiten.Image {
	img := ebiten.NewImage(3, 3)
	img.Fill(color.White)
	return img
}()

func (a *Area) Draw(screen *ebiten.Image) {
	var vertices []ebiten.Vertex
	for i := 0; i < 3; i++ {
		v := ebiten.Vertex{
			DstX: float32(a.Triangle[i].x),
			DstY: float32(a.Triangle[i].y),
			SrcX: 0,
			SrcY: 0,
		}
		switch a.color {
		case -1:
			v.ColorA = 0.0
		case 0:
			v.ColorR = 1.0
			v.ColorA = 0.3
		case 1:
			v.ColorG = 1.0
			v.ColorA = 0.3
		case 2:
			v.ColorB = 1.0
			v.ColorA = 0.3
		case 3:
			v.ColorR = 1.0
			v.ColorG = 1.0
			v.ColorA = 0.3
		}
		vertices = append(vertices, v)
	}
	indices := []uint16{0, 1, 2}
	op := &ebiten.DrawTrianglesOptions{}
	screen.DrawTriangles(vertices, indices, emptyImage.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image), op)

	ebitenutil.DrawLine(screen, a.Triangle[0].x, a.Triangle[0].y, a.Triangle[1].x, a.Triangle[1].y, color.White)
	ebitenutil.DrawLine(screen, a.Triangle[1].x, a.Triangle[1].y, a.Triangle[2].x, a.Triangle[2].y, color.White)
	ebitenutil.DrawLine(screen, a.Triangle[2].x, a.Triangle[2].y, a.Triangle[0].x, a.Triangle[0].y, color.White)
}

type Star struct {
	Point
	r float64
}

func (s *Star) Draw(screen *ebiten.Image) {
	ebitenutil.DrawCircle(screen, s.x, s.y, s.r, color.White)
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
	areas                []Area
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
		}
	case GameModeOpening:
		if g.ticksFromModeStart > 5*60 {
			g.setNextMode(GameModePlaying)
		}
	case GameModePlaying:
		if g.ticksFromModeStart%600 == 0 {
			loggingutil.SendLog(gameName, g.playerID, g.playID, map[string]interface{}{
				"action": "playing",
				"ticks":  g.ticksFromModeStart,
				"score":  g.score,
			})
		}

		if g.touchContext.IsJustTouched() {
			pos := g.touchContext.GetTouchPosition()
			for i := range g.areas {
				a := &g.areas[i]
				if a.Triangle.covers(&Point{x: float64(pos.X), y: float64(pos.Y)}) {
					a.color = (a.color + 1) % 4
					break
				}
			}
		}

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

			g.setNextMode(GameModeGameOver)

			g.rankingCh = loggingutil.RegisterScoreToRankingAsync(gameName, g.playerID, g.playID, g.score)
		}
	case GameModeGameOver:
		if g.ticksFromModeStart > 60 && g.touchContext.IsJustTouched() {
			select {
			case g.ranking = <-g.rankingCh:
			default:
			}

			if len(g.ranking) > 0 {
				g.setNextMode(GameModeRanking)
			} else {
				g.initialize()
			}
		}
	case GameModeRanking:
		if len(g.ranking) == 0 || g.ticksFromModeStart > 60 && g.touchContext.IsJustTouched() {
			g.initialize()
		}
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.Black)

	ebitenutil.DebugPrint(screen, fmt.Sprintf("%.1f", ebiten.ActualFPS()))

	switch g.mode {
	case GameModeOpening:
		for _, s := range g.stars {
			s.Draw(screen)
		}

		index := int(g.ticksFromModeStart / 60)
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
				v = v.mul(float64(g.ticksFromModeStart%60) / 60)
				p := l[0].add(v)
				ebitenutil.DrawLine(screen, l[0].x, l[0].y, p.x, p.y, color.White)
			}
		}
	default:
		for _, s := range g.stars {
			s.Draw(screen)
		}

		for _, a := range g.areas {
			a.Draw(screen)
		}
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
	findLineToExtend := func(triangles []Triangle) (*Line, *Triangle) {
		var line *Line
		var pair *Triangle

		center := Point{x: screenWidth / 2, y: screenHeight / 2}

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

				if l[0].x < 50 || l[0].x > screenWidth-50 ||
					l[0].y < 50 || l[0].y > screenHeight-150 ||
					l[1].x < 50 || l[1].x > screenWidth-50 ||
					l[1].y < 50 || l[1].y > screenHeight-150 {
					continue
				}

				if line == nil || l.distanceSq(&center) < line.distanceSq(&center) {
					line = &l
					pair = &trs[0]
				}
			}
		}

		return line, pair
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
		newPoint.y = math.Min(newPoint.y, screenHeight-5)

		// Ensure the new triangle does not cross over existing triangles
		l1 := Line([2]Point{*newPoint, line[0]})
		l2 := Line([2]Point{*newPoint, line[1]})
		for _, t := range triangles {
			for i := 0; i < 3; i++ {
				l := Line([2]Point{t[i%3], t[(i+1)%3]})
				if l1[1] != l[0] && l1[1] != l[1] && l1.cross(&l) ||
					l2[1] != l[0] && l2[1] != l[1] && l2.cross(&l) {
					return nil
				}
			}
		}

		triangle := Triangle{
			line[0],
			line[1],
			*newPoint,
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

						// Ensure the new triangle does not coss over existing triangles
						collide := false
						line := Line([2]Point{newTriangle[1], newTriangle[2]})
						var trs []Triangle
						trs = append(trs, triangles...)
						trs = append(trs, newTriangles...)
						for _, t := range trs {
							if newTriangle.equals(&t) {
								collide = true
								break
							}
							for i := 0; i < 3; i++ {
								l := Line([2]Point{t[i%3], t[(i+1)%3]})
								if line[0] == l[0] || line[0] == l[1] || line[1] == l[0] || line[1] == l[1] {
									continue
								}
								if line.cross(&l) {
									collide = true
									break
								}
							}
							if collide {
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

		line, pair := findLineToExtend(triangles)
		if line == nil {
			break
		}

		found := false
		for retry := 0; retry < 3; retry++ {
			if t := extendLine(triangles, line, pair); t != nil {
				found = true
				triangles = append(triangles, *t)
				break
			}
		}

		if !found {
			triangles = triangles[:1]
			continue
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
	g.areas = nil
	g.openingLineDrawOrder = nil

	for i := 0; i < 50; i++ {
		g.stars = append(g.stars, Star{
			Point: Point{
				x: screenWidth * g.random.Float64(),
				y: screenHeight * g.random.Float64(),
			},
			r: 1.0 + 0.5*g.random.NormFloat64(),
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

	g.openingLineDrawOrder = [][]Line{{
		Line([2]Point{t0[0], t0[1]}),
		Line([2]Point{t0[1], t0[2]}),
		Line([2]Point{t0[2], t0[0]}),
	}}
	lineExists := func(line *Line, openingLineDrawOrder [][]Line, newLines []Line) bool {
		for _, ls := range openingLineDrawOrder {
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
		lines := g.openingLineDrawOrder[len(g.openingLineDrawOrder)-1]
		var newLines []Line
		for _, a := range g.areas {
			for i := 0; i < 3; i++ {
				for _, line := range lines {
					if line[1] == a.Triangle[i] {
						l1 := Line([2]Point{a.Triangle[i], a.Triangle[(i+1)%3]})
						l2 := Line([2]Point{a.Triangle[i], a.Triangle[(i+2)%3]})
						if !lineExists(&l1, g.openingLineDrawOrder, newLines) {
							newLines = append(newLines, l1)
						}
						if !lineExists(&l2, g.openingLineDrawOrder, newLines) {
							newLines = append(newLines, l2)
						}
					}
				}
			}
		}
		if len(newLines) == 0 {
			break
		}
		g.openingLineDrawOrder = append(g.openingLineDrawOrder, newLines)
	}

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
