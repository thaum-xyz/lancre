# Flow rate

## PID calculations

Flow rate is set with a PID controler based on promQL statement:

```
P * diff + D * deriv(diff[5m]) + I * sum_over_time(diff[5m])
```

Where:

P = multiplier for proportional part
I = multiplier for integral part
D = multiplier for derivative part
diff = temperature difference calculated with a following formula:

```
diff = (S + O)/2 - I
```

Where:

S = solar panel temperature (high rate of change)
O = fluid temperature outgoing from a solar panel, measured inside a tank
I = fluid temparature incoming into a solar panel, measured inside a tank

## Extremes

Due to technical limitations of a solar panel flow rate is restricted by lower and upper bounds. Current restrictions
are as follows:

lower bound: 90 l/min  (TODO(paulfantom): double check this)
upper bound: 210 l/min (TODO(paulfantom): double check this)

## Flow measurement

Flow rate is calculated by a flow meter which counts rotations per minute and from this calculates a flow in l/min.
