# EVOK

The only part which is not managed via ansible and not deployed inside container is [UniPi API - EVOK](https://github.com/UniPiTechnology/evok). This is due to deprecation of installation method needed for unipi v1.1 and lack of debian packages. For this reason evok needs to be installed manually and updated with shel script supplied by evok team.

## Plans and wishes

Due to low number of feature used from evok API it might be possible to revrite evok stack into few modules, each responsible for hardware part. This would allow easier custom integrations with other components (like prometheus). To accomplish such task following hardware needs a software controller implementation:
- MCP23008 to control relays over I2C (address 0x20)
- Digital Inputs
- MCP3422 to read 2 analog inputs over I2C (address 0x68)
- GPIO18 PWM 400Hz analog output
- DS2482-100 to control one-wire devices over I2C (address 0x18)

Full list is available in [evok user manual](https://www.unipi.technology/shop/product/download?fileId=986).

Note: due to unipi 1.1 being still supported all this work is not necessary now.
