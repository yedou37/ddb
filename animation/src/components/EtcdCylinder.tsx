import {Circle, Rect, Txt} from '@motion-canvas/2d';

import {colors} from '../theme/colors';

export interface EtcdCylinderProps {
  x: number;
  y: number;
  width?: number;
  height?: number;
  label?: string;
}

export function EtcdCylinder({
  x,
  y,
  width = 180,
  height = 160,
  label = 'etcd',
}: EtcdCylinderProps) {
  return (
    <>
      <Rect
        x={x}
        y={y}
        width={width}
        height={height}
        fill={colors.etcd}
        opacity={0.16}
        stroke={colors.etcd}
        lineWidth={3}
        radius={12}
      />
      <Circle
        x={x}
        y={y - height / 2}
        width={width}
        height={width}
        scaleY={0.24}
        fill={colors.etcd}
        opacity={0.26}
        stroke={colors.etcd}
        lineWidth={3}
      />
      <Circle
        x={x}
        y={y + height / 2}
        width={width}
        height={width}
        scaleY={0.24}
        fill={colors.backgroundAccent}
        opacity={0.9}
        stroke={colors.etcd}
        lineWidth={3}
      />
      <Txt
        x={x}
        y={y - 6}
        text={label}
        fill={colors.textPrimary}
        fontSize={34}
        fontWeight={700}
      />
      <Txt
        x={x}
        y={y + 34}
        text={'service discovery'}
        fill={colors.textSecondary}
        fontSize={20}
      />
    </>
  );
}
