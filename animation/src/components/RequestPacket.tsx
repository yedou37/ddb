import {Rect, Txt} from '@motion-canvas/2d';

import {colors} from '../theme/colors';

export interface RequestPacketProps {
  x: number;
  y: number;
  title: string;
  detail: string;
  accent: string;
  width?: number;
}

export function RequestPacket({
  x,
  y,
  title,
  detail,
  accent,
  width = 220,
}: RequestPacketProps) {
  return (
    <Rect
      x={x}
      y={y}
      width={width}
      height={70}
      radius={16}
      fill={colors.surface}
      stroke={accent}
      lineWidth={3}
      shadowBlur={14}
      shadowColor={accent}
    >
      <Txt text={title} y={-12} fill={colors.textPrimary} fontSize={18} fontWeight={700} />
      <Txt text={detail} y={12} fill={colors.textSecondary} fontSize={14} />
    </Rect>
  );
}
