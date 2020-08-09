# Analog input math

## Volts to temperature

In case of analog input 1 responsible for solar panel temperature measurements, following math formula is used:

```
(Tmax - Tmin)*volt/Vref + Tmin
```

Parameters are set using documentation or by trial and error:

```
Tmax = 200 degC
Tmin = 0
Vref = 12.0V
```

With such assumption this shortens to `volt * 200/12` which is used in prometheus recording rules as `solar:evok_temperature:`
