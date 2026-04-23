import {Rect, Txt} from '@motion-canvas/2d';

import {colors} from '../theme/colors';

export interface ApiServerBoxProps {
  x: number;
  y: number;
  width?: number;
  height?: number;
  label?: string;
}

export function ApiServerBox({
  x,
  y,
  width = 220,
  height = 100,
  label = 'apiserver',
}: ApiServerBoxProps) {
  return (
    <Rect
      x={x}
      y={y}
      width={width}
      height={height}
      radius={18}
      fill={colors.api}
      opacity={0.2}
      stroke={colors.api}
      lineWidth={3}
      shadowBlur={22}
      shadowColor={colors.api}
    >
      <Txt text={label} fill={colors.textPrimary} fontSize={30} fontWeight={700} />
      <Txt y={28} text={'route / coordinate'} fill={colors.textSecondary} fontSize={18} />
    </Rect>
  );
}
