import {Circle, Line, Rect, Txt} from '@motion-canvas/2d';

import {HashRingSlot} from '../data/hashRing';
import {colors} from '../theme/colors';

export interface HashRingProps {
  x: number;
  y: number;
  radius?: number;
  slots: HashRingSlot[];
}

function slotColor(group: string) {
  switch (group) {
    case 'group 1':
      return colors.shardA;
    case 'group 2':
      return colors.shardB;
    case 'group 3':
      return colors.shardD;
    default:
      return colors.textSecondary;
  }
}

function pointAt(radius: number, angleDeg: number): [number, number] {
  const rad = (angleDeg * Math.PI) / 180;
  return [Math.cos(rad) * radius, Math.sin(rad) * radius];
}

export function HashRing({x, y, radius = 250, slots}: HashRingProps) {
  return (
    <>
      <Circle
        x={x}
        y={y}
        width={radius * 2}
        height={radius * 2}
        stroke={colors.panelStroke}
        lineWidth={10}
        fill={colors.backgroundAccent}
        opacity={0.7}
      />

      <Circle
        x={x}
        y={y}
        width={(radius - 54) * 2}
        height={(radius - 54) * 2}
        fill={colors.background}
        stroke={colors.panelStroke}
        lineWidth={2}
      />

      <Txt
        x={x}
        y={y}
        text={'hash ring'}
        fill={colors.textPrimary}
        fontSize={34}
        fontWeight={700}
      />

      {slots.map((slot) => {
        const [sx, sy] = pointAt(radius - 5, slot.angleDeg);
        const [mx, my] = pointAt(radius - 28, slot.angleDeg);
        const [tx, ty] = pointAt(radius - 28, slot.angleDeg);
        const [lx, ly] = pointAt(radius + 34, slot.angleDeg);
        const c = slotColor(slot.group);
        return (
          <>
            <Circle
              x={x + mx}
              y={y + my}
              width={26}
              height={26}
              fill={colors.surface}
              stroke={c}
              lineWidth={3}
              shadowBlur={10}
              shadowColor={c}
            />
            <Txt
              x={x + tx}
              y={y + ty}
              text={slot.shard}
              fill={colors.textPrimary}
              fontSize={12}
              fontWeight={700}
              rotation={slot.angleDeg + 90}
            />
            <Line
              points={[
                [x + sx, y + sy],
                [x + lx, y + ly],
              ]}
              stroke={c}
              lineWidth={3}
              opacity={0.9}
            />
            <Rect
              x={x + lx}
              y={y + ly}
              width={70}
              height={28}
              radius={8}
              fill={colors.surface}
              stroke={c}
              lineWidth={2}
            >
              <Txt text={slot.shard} fill={colors.textPrimary} fontSize={14} fontWeight={700} />
            </Rect>
          </>
        );
      })}
    </>
  );
}
