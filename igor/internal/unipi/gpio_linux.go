package unipi

import (
	"fmt"

	"periph.io/x/conn/v3/gpio"
	"periph.io/x/host/v3/gpioioctl"
)

// inputLine finds a header GPIO by BCM number on the SoC's GPIO chip. Line
// names differ between kernels (GPIO4 is "GPIO_GCLK"), offsets do not.
func inputLine(bcm int) (gpio.PinIn, error) {
	for _, chip := range gpioioctl.Chips {
		if chip.LineCount() < 28 { // skip the expander chips (gpiochip1)
			continue
		}
		if line := chip.ByNumber(bcm); line != nil {
			return line, nil
		}
	}
	return nil, fmt.Errorf("no GPIO chip with line %d", bcm)
}
