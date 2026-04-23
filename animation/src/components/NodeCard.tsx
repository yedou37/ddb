import {Rect, Txt} from '@motion-canvas/2d';

import {colors} from '../theme/colors';

export interface NodeCardProps {
  x: number;
  y: number;
  label: string;
  leader?: boolean;
  offline?: boolean;
  failed?: boolean;
}

export function NodeCard({
  x,
  y,
  label,
  leader = false,
  offline = false,
  failed = false,
}: NodeCardProps) {
  const stroke = failed
    ? colors.danger
    : offline
      ? colors.offline
      : leader
        ? colors.success
        : colors.panelStroke;
  const fill = failed ? '#34191b' : offline ? '#1b2433' : colors.surfaceMuted;
  const subtitle = failed ? 'failed' : offline ? 'offline' : leader ? 'leader' : 'follower';
  const compactLabel = label.replace(/-node\s+/g, ' / n');

  return (
    <Rect
      x={x}
      y={y}
      width={156}
      height={54}
      radius={14}
      fill={fill}
      stroke={stroke}
      lineWidth={leader ? 4 : 2}
      shadowBlur={leader || failed ? 16 : 0}
      shadowColor={stroke}
    >
      <Txt text={compactLabel} y={-8} fill={colors.textPrimary} fontSize={15} fontWeight={700} />
      <Txt text={subtitle} y={12} fill={colors.textSecondary} fontSize={14} />
    </Rect>
  );
}
