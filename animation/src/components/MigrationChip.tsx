import {Rect, Txt} from '@motion-canvas/2d';

import {colors} from '../theme/colors';

export interface MigrationChipProps {
  x: number;
  y: number;
  label: string;
  accent?: string;
}

export function MigrationChip({
  x,
  y,
  label,
  accent = colors.warning,
}: MigrationChipProps) {
  return (
    <Rect
      x={x}
      y={y}
      width={72}
      height={34}
      radius={12}
      fill={colors.surface}
      stroke={accent}
      lineWidth={3}
      shadowBlur={16}
      shadowColor={accent}
    >
      <Txt text={label} fill={colors.textPrimary} fontSize={16} fontWeight={700} />
    </Rect>
  );
}
