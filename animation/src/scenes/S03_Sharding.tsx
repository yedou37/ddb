import {Layout, Line, Rect, Txt, makeScene2D} from '@motion-canvas/2d';
import {all, createRef, waitFor} from '@motion-canvas/core';

import {RequestPacket} from '../components/RequestPacket';
import {GroupPanel} from '../components/GroupPanel';
import {demoRequests} from '../data/requests';
import {overviewTopology} from '../data/topology';
import {colors} from '../theme/colors';

export default makeScene2D(function* (view) {
  const leftPanelRef = createRef<Layout>();
  const routerRef = createRef<Layout>();
  const mappingRef = createRef<Layout>();
  const groupsRef = createRef<Layout>();

  const packetARef = createRef<Layout>();
  const packetBRef = createRef<Layout>();
  const keyARef = createRef<Layout>();
  const keyBRef = createRef<Layout>();
  const shardARef = createRef<Layout>();
  const shardBRef = createRef<Layout>();
  const targetARef = createRef<Layout>();
  const targetBRef = createRef<Layout>();

  const routeA1 = createRef<Line>();
  const routeA2 = createRef<Line>();
  const routeA3 = createRef<Line>();
  const routeB1 = createRef<Line>();
  const routeB2 = createRef<Line>();
  const routeB3 = createRef<Line>();

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
        text={'Sharding Decision'}
        fill={colors.textPrimary}
        fontSize={56}
        fontWeight={700}
      />
      <Txt
        x={0}
        y={-420}
        text={'payload -> routing key -> shard -> replica group'}
        fill={colors.textSecondary}
        fontSize={26}
      />

      <Layout ref={leftPanelRef} opacity={0}>
        <Rect
          x={-700}
          y={-20}
          width={280}
          height={250}
          radius={24}
          fill={colors.surface}
          stroke={colors.panelStroke}
          lineWidth={3}
        >
          <Txt x={-54} y={-90} text={'input rows'} fill={colors.textPrimary} fontSize={28} fontWeight={700} />
          <Txt x={46} y={-90} text={'same table'} fill={colors.textSecondary} fontSize={18} />
        </Rect>
      </Layout>

      <Layout ref={routerRef} opacity={0}>
        <Rect
          x={-170}
          y={-30}
          width={280}
          height={180}
          radius={26}
          fill={colors.api}
          opacity={0.16}
          stroke={colors.api}
          lineWidth={3}
          shadowBlur={18}
          shadowColor={colors.api}
        >
          <Txt text={'routing'} y={-38} fill={colors.textPrimary} fontSize={34} fontWeight={700} />
          <Txt text={'extract shard key'} y={4} fill={colors.textSecondary} fontSize={22} />
          <Txt text={'compute shard id'} y={36} fill={colors.textSecondary} fontSize={22} />
        </Rect>
      </Layout>

      <Layout ref={mappingRef} opacity={0}>
        <Rect
          x={260}
          y={-30}
          width={290}
          height={180}
          radius={26}
          fill={colors.surface}
          stroke={colors.panelStroke}
          lineWidth={3}
        >
          <Txt text={'shard mapping'} y={-40} fill={colors.textPrimary} fontSize={32} fontWeight={700} />
          <Txt text={'S2 -> group 1'} y={0} fill={colors.shardA} fontSize={28} fontWeight={700} />
          <Txt text={'S9 -> group 3'} y={38} fill={colors.shardD} fontSize={28} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={groupsRef} opacity={0}>
        <Layout scale={0.75}>
          <GroupPanel
            x={-250}
            y={285}
            groupId={overviewTopology[0].id}
            leaderId={overviewTopology[0].leader}
            nodes={overviewTopology[0].nodes}
            shards={overviewTopology[0].shards}
          />
          <GroupPanel
            x={250}
            y={285}
            groupId={overviewTopology[2].id}
            leaderId={overviewTopology[2].leader}
            nodes={overviewTopology[2].nodes}
            shards={overviewTopology[2].shards}
          />
        </Layout>
      </Layout>

      <Layout ref={packetARef} opacity={0}>
        <RequestPacket
          x={-700}
          y={-70}
          title={demoRequests[0].label}
          detail={demoRequests[0].detail}
          accent={colors.shardA}
        />
      </Layout>

      <Layout ref={packetBRef} opacity={0}>
        <RequestPacket
          x={-700}
          y={40}
          title={demoRequests[1].label}
          detail={demoRequests[1].detail}
          accent={colors.shardD}
        />
      </Layout>

      <Layout ref={keyARef} opacity={0}>
        <Rect
          x={-170}
          y={-76}
          width={220}
          height={54}
          radius={14}
          fill={colors.surface}
          stroke={colors.shardA}
          lineWidth={3}
        >
          <Txt text={'key 7 -> shard S2'} fill={colors.textPrimary} fontSize={20} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={keyBRef} opacity={0}>
        <Rect
          x={-170}
          y={34}
          width={244}
          height={54}
          radius={14}
          fill={colors.surface}
          stroke={colors.shardD}
          lineWidth={3}
        >
          <Txt text={'key 101 -> shard S9'} fill={colors.textPrimary} fontSize={20} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={shardARef} opacity={0}>
        <Rect
          x={260}
          y={-76}
          width={170}
          height={54}
          radius={14}
          fill={colors.surface}
          stroke={colors.shardA}
          lineWidth={3}
        >
          <Txt text={'S2 -> group 1'} fill={colors.textPrimary} fontSize={20} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={shardBRef} opacity={0}>
        <Rect
          x={260}
          y={34}
          width={170}
          height={54}
          radius={14}
          fill={colors.surface}
          stroke={colors.shardD}
          lineWidth={3}
        >
          <Txt text={'S9 -> group 3'} fill={colors.textPrimary} fontSize={20} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={targetARef} opacity={0}>
        <Rect
          x={-190}
          y={430}
          width={220}
          height={56}
          radius={14}
          fill={colors.surface}
          stroke={colors.shardA}
          lineWidth={3}
        >
          <Txt text={'route to group 1 leader'} fill={colors.textPrimary} fontSize={20} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={targetBRef} opacity={0}>
        <Rect
          x={190}
          y={430}
          width={220}
          height={56}
          radius={14}
          fill={colors.surface}
          stroke={colors.shardD}
          lineWidth={3}
        >
          <Txt text={'route to group 3 leader'} fill={colors.textPrimary} fontSize={20} fontWeight={700} />
        </Rect>
      </Layout>

      <Line
        ref={routeA1}
        points={[
          [-580, -70],
          [-290, -76],
        ]}
        lineWidth={5}
        stroke={colors.shardA}
        endArrow
        end={0}
      />
      <Line
        ref={routeA2}
        points={[
          [-60, -76],
          [168, -76],
        ]}
        lineWidth={5}
        stroke={colors.shardA}
        endArrow
        end={0}
      />
      <Line
        ref={routeA3}
        points={[
          [350, -50],
          [100, 170],
          [-180, 402],
        ]}
        lineWidth={5}
        stroke={colors.shardA}
        endArrow
        end={0}
      />

      <Line
        ref={routeB1}
        points={[
          [-580, 40],
          [-290, 34],
        ]}
        lineWidth={5}
        stroke={colors.shardD}
        endArrow
        end={0}
      />
      <Line
        ref={routeB2}
        points={[
          [-48, 34],
          [168, 34],
        ]}
        lineWidth={5}
        stroke={colors.shardD}
        endArrow
        end={0}
      />
      <Line
        ref={routeB3}
        points={[
          [350, 60],
          [430, 180],
          [250, 402],
        ]}
        lineWidth={5}
        stroke={colors.shardD}
        endArrow
        end={0}
      />
    </>,
  );

  yield* all(
    leftPanelRef().opacity(1, 0.7),
    routerRef().opacity(1, 0.7),
    mappingRef().opacity(1, 0.7),
    groupsRef().opacity(1, 0.8),
  );
  yield* waitFor(0.3);

  yield* all(packetARef().opacity(1, 0.4), packetBRef().opacity(1, 0.4));
  yield* all(routeA1().end(1, 0.85), routeB1().end(1, 0.85));
  yield* all(keyARef().opacity(1, 0.4), keyBRef().opacity(1, 0.4));
  yield* waitFor(0.25);

  yield* all(routeA2().end(1, 0.8), routeB2().end(1, 0.8));
  yield* all(shardARef().opacity(1, 0.4), shardBRef().opacity(1, 0.4));
  yield* waitFor(0.25);

  yield* all(routeA3().end(1, 1.0), routeB3().end(1, 1.0));
  yield* all(targetARef().opacity(1, 0.4), targetBRef().opacity(1, 0.4));

  yield* waitFor(1.1);
});
