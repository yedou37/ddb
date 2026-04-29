import {Layout, Line, Rect, Txt, makeScene2D} from '@motion-canvas/2d';
import {all, createRef, waitFor} from '@motion-canvas/core';

import {GroupPanel} from '../components/GroupPanel';
import {RequestPacket} from '../components/RequestPacket';
import {overviewTopology} from '../data/topology';
import {colors} from '../theme/colors';
import {flashTwice} from '../utils/animation';

export default makeScene2D(function* (view) {
  const groupPos: [number, number] = [300, 220];
  const node1Pos: [number, number] = [groupPos[0] - 118, groupPos[1] - 20];
  const node2Pos: [number, number] = [groupPos[0], groupPos[1] - 20];
  const node3Pos: [number, number] = [groupPos[0] + 118, groupPos[1] - 20];
  const shardS2Pos: [number, number] = [groupPos[0] + 46, groupPos[1] + 96];

  const logicBandRef = createRef<Layout>();
  const physicalBandRef = createRef<Layout>();
  const rowRef = createRef<Layout>();
  const hashRef = createRef<Layout>();
  const shardRef = createRef<Layout>();
  const ownerRef = createRef<Layout>();
  const groupRef = createRef<Layout>();
  const shardHighlightRef = createRef<Layout>();
  const node1HighlightRef = createRef<Layout>();
  const node2HighlightRef = createRef<Layout>();
  const node3HighlightRef = createRef<Layout>();
  const leaderBadgeRef = createRef<Layout>();
  const replicaBadgeRef = createRef<Layout>();
  const summaryRef = createRef<Layout>();

  const rowToHashRef = createRef<Line>();
  const hashToShardRef = createRef<Line>();
  const shardToOwnerRef = createRef<Line>();
  const ownerToGroupRef = createRef<Line>();

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
        text={'Row, Shard, Group, Node'}
        fill={colors.textPrimary}
        fontSize={56}
        fontWeight={700}
      />
      <Txt
        x={0}
        y={-420}
        text={'logical mapping lives on the top row, physical replication lives on the bottom row'}
        fill={colors.textSecondary}
        fontSize={24}
      />

      <Layout ref={logicBandRef} opacity={0}>
        <Rect
          x={0}
          y={-118}
          width={1700}
          height={260}
          radius={30}
          fill={colors.surface}
          stroke={colors.panelStroke}
          lineWidth={3}
          opacity={0.35}
        />
        <Txt text={'logical mapping'} x={-732} y={-222} fill={colors.textPrimary} fontSize={30} fontWeight={700} />
        <Txt text={'one row is routed to one shard, then to one owner group'} x={-500} y={-186} fill={colors.textSecondary} fontSize={18} />
      </Layout>

      <Layout ref={physicalBandRef} opacity={0}>
        <Rect
          x={0}
          y={214}
          width={1700}
          height={422}
          radius={30}
          fill={colors.surface}
          stroke={colors.panelStroke}
          lineWidth={3}
          opacity={0.28}
        />
        <Txt text={'physical replication'} x={-728} y={14} fill={colors.textPrimary} fontSize={30} fontWeight={700} />
        <Txt text={'one owner group contains a leader and follower nodes'} x={-412} y={46} fill={colors.textSecondary} fontSize={18} />

        <Rect
          x={-540}
          y={184}
          width={310}
          height={222}
          radius={24}
          fill={colors.backgroundAccent}
          stroke={colors.panelStroke}
          lineWidth={2}
          opacity={0.86}
        >
          <Txt text={'how to read it'} y={-76} fill={colors.textPrimary} fontSize={28} fontWeight={700} />
          <Txt text={'1 row -> 1 shard'} y={-24} fill={colors.shardA} fontSize={24} fontWeight={700} />
          <Txt text={'1 shard -> 1 owner group'} y={18} fill={colors.warning} fontSize={24} fontWeight={700} />
          <Txt text={'1 group -> many raft nodes'} y={60} fill={colors.success} fontSize={24} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={rowRef} opacity={0}>
        <RequestPacket
          x={-640}
          y={-118}
          title={'row: user_id = 7'}
          detail={'one table row enters the system'}
          accent={colors.request}
          width={254}
        />
      </Layout>

      <Line
        ref={rowToHashRef}
        points={[
          [-512, -118],
          [-406, -118],
          [-302, -118],
        ]}
        lineWidth={6}
        stroke={colors.request}
        endArrow
        end={0}
      />

      <Layout ref={hashRef} opacity={0}>
        <Rect
          x={-184}
          y={-118}
          width={246}
          height={112}
          radius={22}
          fill={colors.api}
          opacity={0.16}
          stroke={colors.api}
          lineWidth={3}
          shadowBlur={18}
          shadowColor={colors.api}
        >
          <Txt text={'sharding'} y={-18} fill={colors.textPrimary} fontSize={30} fontWeight={700} />
          <Txt text={'extract key -> hash'} y={16} fill={colors.textSecondary} fontSize={19} />
        </Rect>
      </Layout>

      <Line
        ref={hashToShardRef}
        points={[
          [-60, -118],
          [40, -118],
          [138, -118],
        ]}
        lineWidth={6}
        stroke={colors.api}
        endArrow
        end={0}
      />

      <Layout ref={shardRef} opacity={0}>
        <Rect
          x={252}
          y={-118}
          width={194}
          height={92}
          radius={20}
          fill={colors.surface}
          stroke={colors.shardA}
          lineWidth={3}
          shadowBlur={14}
          shadowColor={colors.shardA}
        >
          <Txt text={'shard S2'} y={-10} fill={colors.textPrimary} fontSize={28} fontWeight={700} />
          <Txt text={'logical partition'} y={18} fill={colors.textSecondary} fontSize={16} />
        </Rect>
      </Layout>

      <Line
        ref={shardToOwnerRef}
        points={[
          [350, -118],
          [448, -118],
          [548, -118],
        ]}
        lineWidth={6}
        stroke={colors.shardA}
        endArrow
        end={0}
      />

      <Layout ref={ownerRef} opacity={0}>
        <Rect
          x={678}
          y={-118}
          width={250}
          height={104}
          radius={22}
          fill={colors.surface}
          stroke={colors.warning}
          lineWidth={3}
          shadowBlur={14}
          shadowColor={colors.warning}
        >
          <Txt text={'owner: group 1'} y={-14} fill={colors.textPrimary} fontSize={28} fontWeight={700} />
          <Txt text={'S2 currently belongs here'} y={18} fill={colors.textSecondary} fontSize={17} />
        </Rect>
      </Layout>

      <Line
        ref={ownerToGroupRef}
        points={[
          [678, -64],
          [678, 54],
          [520, 118],
          [388, 154],
        ]}
        lineWidth={6}
        stroke={colors.warning}
        endArrow
        end={0}
      />

      <Layout ref={groupRef} opacity={0}>
        <GroupPanel
          x={groupPos[0]}
          y={groupPos[1]}
          groupId={overviewTopology[0].id}
          leaderId={overviewTopology[0].leader}
          nodes={overviewTopology[0].nodes}
          shards={overviewTopology[0].shards}
        />
      </Layout>

      <Layout ref={shardHighlightRef} opacity={0}>
        <Rect
          x={shardS2Pos[0]}
          y={shardS2Pos[1]}
          width={88}
          height={44}
          radius={12}
          fill={colors.shardA}
          opacity={0.18}
          stroke={colors.shardA}
          lineWidth={3}
          shadowBlur={18}
          shadowColor={colors.shardA}
        />
      </Layout>

      <Layout ref={node1HighlightRef} opacity={0}>
        <Rect
          x={node1Pos[0]}
          y={node1Pos[1]}
          width={168}
          height={66}
          radius={16}
          fill={colors.success}
          opacity={0.14}
          stroke={colors.success}
          lineWidth={3}
          shadowBlur={18}
          shadowColor={colors.success}
        />
      </Layout>

      <Layout ref={node2HighlightRef} opacity={0}>
        <Rect
          x={node2Pos[0]}
          y={node2Pos[1]}
          width={168}
          height={66}
          radius={16}
          fill={colors.api}
          opacity={0.12}
          stroke={colors.api}
          lineWidth={3}
          shadowBlur={16}
          shadowColor={colors.api}
        />
      </Layout>

      <Layout ref={node3HighlightRef} opacity={0}>
        <Rect
          x={node3Pos[0]}
          y={node3Pos[1]}
          width={168}
          height={66}
          radius={16}
          fill={colors.api}
          opacity={0.12}
          stroke={colors.api}
          lineWidth={3}
          shadowBlur={16}
          shadowColor={colors.api}
        />
      </Layout>

      <Layout ref={leaderBadgeRef} opacity={0}>
        <Rect
          x={node1Pos[0]}
          y={node1Pos[1] - 72}
          width={216}
          height={50}
          radius={14}
          fill={colors.surface}
          stroke={colors.success}
          lineWidth={3}
        >
          <Txt text={'leader handles writes'} fill={colors.textPrimary} fontSize={19} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={replicaBadgeRef} opacity={0}>
        <Rect
          x={node2Pos[0] + 58}
          y={node2Pos[1] + 94}
          width={320}
          height={56}
          radius={16}
          fill={colors.surface}
          stroke={colors.api}
          lineWidth={3}
        >
          <Txt text={'followers copy the shard data'} fill={colors.textPrimary} fontSize={18} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={summaryRef} opacity={0}>
        <Rect
          x={0}
          y={448}
          width={1280}
          height={82}
          radius={22}
          fill={colors.surface}
          stroke={colors.panelStroke}
          lineWidth={3}
          opacity={0.95}
        />
        <Txt text={'logical: row -> shard -> owner group'} x={-250} y={448} fill={colors.warning} fontSize={26} fontWeight={700} />
        <Txt text={'physical: group -> leader + followers'} x={270} y={448} fill={colors.success} fontSize={26} fontWeight={700} />
      </Layout>
    </>,
  );

  yield* all(logicBandRef().opacity(1, 0.65), physicalBandRef().opacity(1, 0.65));
  yield* waitFor(0.2);

  yield* rowRef().opacity(1, 0.4);
  yield* waitFor(0.18);
  yield* rowToHashRef().end(1, 0.6);
  yield* hashRef().opacity(1, 0.42);
  yield* waitFor(0.2);
  yield* hashToShardRef().end(1, 0.55);
  yield* shardRef().opacity(1, 0.38);
  yield* flashTwice(shardRef(), 1, 0.6, 0.14, 0.1, 0.08);
  yield* waitFor(0.18);
  yield* shardToOwnerRef().end(1, 0.55);
  yield* ownerRef().opacity(1, 0.42);
  yield* flashTwice(ownerRef(), 1, 0.62, 0.14, 0.1, 0.08);
  yield* waitFor(0.28);

  yield* ownerToGroupRef().end(1, 0.72);
  yield* groupRef().opacity(1, 0.72);
  yield* waitFor(0.2);
  yield* shardHighlightRef().opacity(1, 0.3);
  yield* flashTwice(shardHighlightRef(), 1, 0.3, 0.14, 0.1, 0.1);
  yield* waitFor(0.25);

  yield* all(node1HighlightRef().opacity(1, 0.28), leaderBadgeRef().opacity(1, 0.3));
  yield* waitFor(0.22);
  yield* node2HighlightRef().opacity(1, 0.28);
  yield* waitFor(0.16);
  yield* node3HighlightRef().opacity(1, 0.28);
  yield* replicaBadgeRef().opacity(1, 0.32);
  yield* flashTwice(replicaBadgeRef(), 1, 0.56, 0.14, 0.1, 0.1);
  yield* waitFor(0.35);

  yield* summaryRef().opacity(1, 0.55);
  yield* waitFor(2.1);
});
