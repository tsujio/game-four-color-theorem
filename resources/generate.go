package main

import (
	"os"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/tsujio/game-util/resourceutil"
)

//go:generate go run generate.go "color-0.wav"
//go:generate go run generate.go "color-1.wav"
//go:generate go run generate.go "color-2.wav"
//go:generate go run generate.go "color-3.wav"
//go:generate go run generate.go "魔王魂 効果音 物音15.mp3"
//go:generate go run generate.go "魔王魂 効果音 システム49.mp3"
//go:generate go run generate.go "魔王魂 効果音 笛01.mp3"

func main() {
	resourceutil.ForceSaveDecodedAudio(os.Args[1], audio.NewContext(48000))
}
