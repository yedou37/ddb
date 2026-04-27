import {waitFor} from '@motion-canvas/core';

type OpacityTarget = {
  opacity: (value: number, duration?: number) => any;
};

export function* flashTwice(
  target: OpacityTarget,
  high = 1,
  low = 0.5,
  rise = 0.18,
  dip = 0.12,
  settle = 0.16,
) {
  yield* target.opacity(high, rise);
  yield* waitFor(settle);
  yield* target.opacity(low, dip);
  yield* target.opacity(high, rise);
  yield* waitFor(settle);
}
