//go:build !linux

package unipi

import (
	"errors"

	"periph.io/x/conn/v3/gpio"
)

func inputLine(int) (gpio.PinIn, error) {
	return nil, errors.New("GPIO needs Linux")
}
