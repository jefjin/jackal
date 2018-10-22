/*
 * Copyright (c) 2018 Miguel Ángel Ortuño.
 * See the LICENSE file for more information.
 */

package main

import (
	"fmt"
	"os"

	"github.com/ortuman/jackal/application"
)

func main() {
	if err := application.New(os.Stdout).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(-1)
	}
}
