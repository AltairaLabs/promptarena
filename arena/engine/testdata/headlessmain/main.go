// Command headlessmain is a program that imports arena/engine and nothing
// else, the way a headless worker does. TestEngine_HeadlessBuildIsStatic
// builds it.
package main

import _ "github.com/AltairaLabs/promptarena/v2/arena/engine"

func main() {}
