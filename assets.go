package main

import _ "embed"

//go:embed assets/flame1.png
var flame1PNG []byte

//go:embed assets/flame2.png
var flame2PNG []byte

//go:embed assets/flame3.png
var flame3PNG []byte

//go:embed assets/flame4.png
var flame4PNG []byte

//go:embed assets/check.png
var checkPNG []byte

//go:embed assets/bang.png
var bangPNG []byte

var flameFrames = [][]byte{flame1PNG, flame2PNG, flame3PNG, flame4PNG}
