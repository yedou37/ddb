import {Circle, Line, Rect, Txt, makeScene2D} from '@motion-canvas/2d';
import {all, createRef, waitFor} from '@motion-canvas/core';

import {HashRing} from '../components/HashRing';
import {RequestPacket} from '../components/RequestPacket';
import {demoRequests} from '../data/requests';
import {hashRingSlots} from '../data/hashRing';
import {colors} from '../theme/colors';

function pointAt(radius: number, angleDeg: number): [number, number] {
  const rad = (angleDeg * Math.PI) / 180;
  return [Math.cos(rad) * radius, Math.sin(rad) * radius];
}

export default makeScene2D(function* (view) {
  const radius = 235;
  const ringX = -120;
  const ringY = 20;

  const slotA = hashRingSlots.find((slot) => slot.shard === demoRequests[0].shard)!;
  const slotB = hashRingSlots.find((slot) => slot.shard === demoRequests[1].shard)!;

  const [ringAx, ringAy] = pointAt(radius - 12, slotA.angleDeg);
  const [ringBx, ringBy] = pointAt(radius - 12, slotB.angleDeg);
  const [labelAx, labelAy] = pointAt(radius + 34, slotA.angleDeg);
  const [labelBx, labelBy] = pointAt(radius + 34, slotB.angleDeg);

  const hashTagARef = createRef<Rect>();
  const hashTagBRef = createRef<Rect>();
  const hitTagARef = createRef<Rect>();
  const hitTagBRef = createRef<Rect>();
  const ownerTagARef = createRef<Rect>();
  const ownerTagBRef = createRef<Rect>();

  const routeARef = createRef<Line>();
  const routeBRef = createRef<Line>();
  const focusARef = createRef<Circle>();
  const focusBRef = createRef<Circle>();
  const pulseARef = createRef<Circle>();
  const pulseBRef = createRef<Circle>();

  view.add(
    <>
      <Rect width={1920} height={1080} fill={colors.background} />
      <Rect
        x={0}
        y={0}
        width={1800}
        height={960}
        radius={42}
        fill={colors.backgroundAccent}
        opacity={0.35}
      />

      <Txt
        x={0}
        y={-470}
        text={'Consistent Hash Ring'}
        fill={colors.textPrimary}
        fontSize={56}
        fontWeight={700}
      />
      <Txt
        x={0}
        y={-420}
        text={'two keys land at different ring positions and resolve to different shard owners'}
        fill={colors.textSecondary}
        fontSize={24}
      />

      <RequestPacket
        x={-720}
        y={-86}
        title={demoRequests[0].label}
        detail={demoRequests[0].detail}
        accent={colors.shardA}
      />
      <RequestPacket
        x={-720}
        y={34}
        title={demoRequests[1].label}
        detail={demoRequests[1].detail}
        accent={colors.shardD}
      />

      <HashRing x={ringX} y={ringY} radius={radius} slots={hashRingSlots} />

      <Rect
        x={560}
        y={0}
        width={420}
        height={460}
        radius={28}
        fill={colors.surface}
        stroke={colors.panelStroke}
        lineWidth={3}
      >
        <Txt x={0} y={-180} text={'lookup steps'} fill={colors.textPrimary} fontSize={34} fontWeight={700} />
        <Txt
          x={0}
          y={-134}
          text={'payload -> hash token -> ring position -> clockwise owner'}
          fill={colors.textSecondary}
          fontSize={18}
        />
      </Rect>

      <Rect
        ref={hashTagARef}
        x={560}
        y={-78}
        width={280}
        height={56}
        radius={14}
        fill={colors.surface}
        stroke={colors.shardA}
        lineWidth={3}
        opacity={0}
      >
        <Txt
          text={`key=7 -> token ${demoRequests[0].hashToken}`}
          fill={colors.textPrimary}
          fontSize={20}
          fontWeight={700}
        />
      </Rect>

      <Rect
        ref={hitTagARef}
        x={560}
        y={-10}
        width={280}
        height={56}
        radius={14}
        fill={colors.surface}
        stroke={colors.shardA}
        lineWidth={3}
        opacity={0}
      >
        <Txt text={'hit shard S2 on ring'} fill={colors.textPrimary} fontSize={20} fontWeight={700} />
      </Rect>

      <Rect
        ref={ownerTagARef}
        x={560}
        y={58}
        width={280}
        height={56}
        radius={14}
        fill={colors.surface}
        stroke={colors.shardA}
        lineWidth={3}
        opacity={0}
      >
        <Txt text={'owner group 1'} fill={colors.textPrimary} fontSize={20} fontWeight={700} />
      </Rect>

      <Rect
        ref={hashTagBRef}
        x={560}
        y={162}
        width={300}
        height={56}
        radius={14}
        fill={colors.surface}
        stroke={colors.shardD}
        lineWidth={3}
        opacity={0}
      >
        <Txt
          text={`key=101 -> token ${demoRequests[1].hashToken}`}
          fill={colors.textPrimary}
          fontSize={20}
          fontWeight={700}
        />
      </Rect>

      <Rect
        ref={hitTagBRef}
        x={560}
        y={230}
        width={300}
        height={56}
        radius={14}
        fill={colors.surface}
        stroke={colors.shardD}
        lineWidth={3}
        opacity={0}
      >
        <Txt text={'hit shard S9 on ring'} fill={colors.textPrimary} fontSize={20} fontWeight={700} />
      </Rect>

      <Rect
        ref={ownerTagBRef}
        x={560}
        y={298}
        width={300}
        height={56}
        radius={14}
        fill={colors.surface}
        stroke={colors.shardD}
        lineWidth={3}
        opacity={0}
      >
        <Txt text={'owner group 3'} fill={colors.textPrimary} fontSize={20} fontWeight={700} />
      </Rect>

      <Line
        ref={routeARef}
        points={[
          [-610, -86],
          [-420, -86],
          [ringX + ringAx, ringY + ringAy],
        ]}
        lineWidth={5}
        stroke={colors.shardA}
        endArrow
        end={0}
      />

      <Line
        ref={routeBRef}
        points={[
          [-610, 34],
          [-420, 34],
          [ringX + ringBx, ringY + ringBy],
        ]}
        lineWidth={5}
        stroke={colors.shardD}
        endArrow
        end={0}
      />

      <Circle
        ref={pulseARef}
        x={ringX + ringAx}
        y={ringY + ringAy}
        width={24}
        height={24}
        fill={colors.shardA}
        shadowBlur={18}
        shadowColor={colors.shardA}
        opacity={0}
      />

      <Circle
        ref={pulseBRef}
        x={ringX + ringBx}
        y={ringY + ringBy}
        width={24}
        height={24}
        fill={colors.shardD}
        shadowBlur={18}
        shadowColor={colors.shardD}
        opacity={0}
      />

      <Circle
        ref={focusARef}
        x={ringX + labelAx}
        y={ringY + labelAy}
        width={98}
        height={48}
        stroke={colors.shardA}
        lineWidth={4}
        opacity={0}
      />

      <Circle
        ref={focusBRef}
        x={ringX + labelBx}
        y={ringY + labelBy}
        width={104}
        height={48}
        stroke={colors.shardD}
        lineWidth={4}
        opacity={0}
      />
    </>,
  );

  yield* all(routeARef().end(1, 0.8), hashTagARef().opacity(1, 0.3));
  yield* pulseARef().opacity(1, 0.2);
  yield* all(focusARef().opacity(1, 0.25), hitTagARef().opacity(1, 0.25));
  yield* ownerTagARef().opacity(1, 0.25);

  yield* waitFor(0.4);

  yield* all(routeBRef().end(1, 0.8), hashTagBRef().opacity(1, 0.3));
  yield* pulseBRef().opacity(1, 0.2);
  yield* all(focusBRef().opacity(1, 0.25), hitTagBRef().opacity(1, 0.25));
  yield* ownerTagBRef().opacity(1, 0.25);

  yield* waitFor(0.8);
});
