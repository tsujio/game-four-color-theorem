package main

import (
	"embed"
	"image/color"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	logging "github.com/tsujio/game-logging-server/client"
	"github.com/tsujio/game-util/loggingutil"
	"github.com/tsujio/game-util/resourceutil"
	"github.com/tsujio/game-util/touchutil"
)

const (
	gameName     = "four-color-theorem"
	screenWidth  = 640
	screenHeight = 480
)

//go:embed resources/*.ttf resources/*.dat resources/bgm-*.wav resources/secret
var resources embed.FS

var (
	fontL, fontM, fontS = resourceutil.ForceLoadFont(resources, "resources/PressStart2P-Regular.ttf", nil)
	audioContext        = audio.NewContext(48000)
)

type GameMode int

const (
	GameModeTitle GameMode = iota
	GameModePlaying
	GameModeGameOver
	GameModeRanking
)

type Game struct {
	playerID           string
	playID             string
	fixedRandomSeed    int64
	touchContext       *touchutil.TouchContext
	random             *rand.Rand
	mode               GameMode
	ticksFromModeStart uint64
	score              int
	rankingCh          <-chan []logging.GameScore
	ranking            []logging.GameScore
}

func (g *Game) Update() error {
	g.touchContext.Update()

	g.ticksFromModeStart++

	loggingutil.SendTouchLog(gameName, g.playerID, g.playID, g.ticksFromModeStart, g.touchContext)

	switch g.mode {
	case GameModeTitle:
		g.setNextMode(GameModePlaying)

		loggingutil.SendLog(gameName, g.playerID, g.playID, map[string]interface{}{
			"action": "start_game",
		})
	case GameModePlaying:
		if g.ticksFromModeStart%600 == 0 {
			loggingutil.SendLog(gameName, g.playerID, g.playID, map[string]interface{}{
				"action": "playing",
				"ticks":  g.ticksFromModeStart,
				"score":  g.score,
			})
		}

		if false {
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
	screen.Fill(color.RGBA{0xc7, 0xd7, 0xc7, 0xff})
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func (g *Game) setNextMode(mode GameMode) {
	g.mode = mode
	g.ticksFromModeStart = 0
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
