import {Rect, Txt} from '@motion-canvas/2d';

import {colors} from '../theme/colors';

export interface ShardChipProps {
  x: number;
  y: number;
  label: string;
  color: string;
}

export function ShardChip({x, y, label, color}: ShardChipProps) {
  return (
    <Rect
      x={x}
      y={y}
      width={72}
      height={34}
      radius={12}
      fill={color}
      opacity={0.22}
      stroke={color}
      lineWidth={2}
    >
      <Txt text={label} fill={colors.textPrimary} fontSize={16} fontWeight={700} />
    </Rect>
  );
}
