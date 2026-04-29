import {Layout, Rect, Txt, makeScene2D} from '@motion-canvas/2d';
import {all, createRef, waitFor} from '@motion-canvas/core';

import {GroupPanel} from '../components/GroupPanel';
import {MigrationChip} from '../components/MigrationChip';
import {rebalanceAfter, rebalanceBefore, rebalanceMoves} from '../data/rebalance';
import {colors} from '../theme/colors';
import {flashTwice} from '../utils/animation';

export default makeScene2D(function* (view) {
  const clusterScale = 0.68;
  const group1Pos: [number, number] = [-360, -100];
  const group2Pos: [number, number] = [-360, 220];
  const group3Pos: [number, number] = [420, -100];
  const group4Pos: [number, number] = [420, 220];

  const centerStagePositions: [number, number][] = [
    [-52, 48],
    [52, 48],
  ];

  const slotPosition = (panel: [number, number], index: number): [number, number] => {
    const localX = -118 + (index % 4) * 82;
    const localY = 96 + Math.floor(index / 4) * 42;
    return [(panel[0] + localX) * clusterScale, (panel[1] + localY) * clusterScale];
  };

  const sourcePositions: [number, number][] = [
    slotPosition(group1Pos, 2),
    slotPosition(group2Pos, 2),
  ];
  const targetPositions: [number, number][] = [
    slotPosition(group4Pos, 0),
    slotPosition(group4Pos, 1),
  ];

  const controlRef = createRef<Layout>();
  const ownershipCardRef = createRef<Layout>();
  const moveListRef = createRef<Layout>();
  const stageRef = createRef<Layout>();
  const joinBadgeRef = createRef<Layout>();
  const joinNode1Ref = createRef<Layout>();
  const joinNode2Ref = createRef<Layout>();
  const joinNode3Ref = createRef<Layout>();
  const clusterRef = createRef<Layout>();
  const group1BeforeRef = createRef<Layout>();
  const group1AfterRef = createRef<Layout>();
  const group2BeforeRef = createRef<Layout>();
  const group2AfterRef = createRef<Layout>();
  const group3Ref = createRef<Layout>();
  const group4GhostRef = createRef<Layout>();
  const group4AfterRef = createRef<Layout>();

  const move1Ref = createRef<Layout>();
  const move2Ref = createRef<Layout>();

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
        text={'Rebalance Flow'}
        fill={colors.textPrimary}
        fontSize={56}
        fontWeight={700}
      />
      <Txt
        x={0}
        y={-420}
        text={'the cluster expands from three groups to four, then shards rebalance into a 2 / 2 / 2 / 2 layout'}
        fill={colors.textSecondary}
        fontSize={22}
      />

      <Layout ref={clusterRef} opacity={0}>
        <Layout scale={clusterScale}>
          <Layout ref={group1BeforeRef} opacity={1}>
            <GroupPanel
              x={group1Pos[0]}
              y={group1Pos[1]}
              groupId={rebalanceBefore[0].id}
              leaderId={rebalanceBefore[0].leader}
              nodes={rebalanceBefore[0].nodes}
              shards={rebalanceBefore[0].shards}
            />
          </Layout>
          <Layout ref={group1AfterRef} opacity={0}>
            <GroupPanel
              x={group1Pos[0]}
              y={group1Pos[1]}
              groupId={rebalanceAfter[0].id}
              leaderId={rebalanceAfter[0].leader}
              nodes={rebalanceAfter[0].nodes}
              shards={rebalanceAfter[0].shards}
            />
          </Layout>

          <Layout ref={group2BeforeRef} opacity={1}>
            <GroupPanel
              x={group2Pos[0]}
              y={group2Pos[1]}
              groupId={rebalanceBefore[1].id}
              leaderId={rebalanceBefore[1].leader}
              nodes={rebalanceBefore[1].nodes}
              shards={rebalanceBefore[1].shards}
            />
          </Layout>
          <Layout ref={group2AfterRef} opacity={0}>
            <GroupPanel
              x={group2Pos[0]}
              y={group2Pos[1]}
              groupId={rebalanceAfter[1].id}
              leaderId={rebalanceAfter[1].leader}
              nodes={rebalanceAfter[1].nodes}
              shards={rebalanceAfter[1].shards}
            />
          </Layout>

          <Layout ref={group3Ref} opacity={1}>
            <GroupPanel
              x={group3Pos[0]}
              y={group3Pos[1]}
              groupId={rebalanceAfter[2].id}
              leaderId={rebalanceAfter[2].leader}
              nodes={rebalanceAfter[2].nodes}
              shards={rebalanceAfter[2].shards}
            />
          </Layout>

          <Layout ref={group4GhostRef} opacity={0}>
            <GroupPanel
              x={group4Pos[0]}
              y={group4Pos[1]}
              groupId={rebalanceAfter[3].id}
              leaderId={rebalanceAfter[3].leader}
              nodes={rebalanceAfter[3].nodes}
              shards={[]}
            />
          </Layout>
          <Layout ref={group4AfterRef} opacity={0}>
            <GroupPanel
              x={group4Pos[0]}
              y={group4Pos[1]}
              groupId={rebalanceAfter[3].id}
              leaderId={rebalanceAfter[3].leader}
              nodes={rebalanceAfter[3].nodes}
              shards={rebalanceAfter[3].shards}
            />
          </Layout>
        </Layout>
      </Layout>

      <Layout ref={controlRef} opacity={0}>
        <Txt text={'rebalance'} x={0} y={-336} fill={colors.textPrimary} fontSize={34} fontWeight={700} />
        <Txt text={'expand with group 4'} x={0} y={-304} fill={colors.textSecondary} fontSize={20} />
        <Txt text={'3 / 3 / 2  ->  2 / 2 / 2 / 2'} x={0} y={-276} fill={colors.textSecondary} fontSize={18} />
      </Layout>

      <Layout ref={joinBadgeRef} opacity={0}>
        <Txt text={'new group 4 joins'} x={group4Pos[0] * clusterScale} y={58} fill={colors.api} fontSize={22} fontWeight={700} />
      </Layout>

      <Layout ref={joinNode1Ref} opacity={0}>
        <Rect
          x={group4Pos[0] * clusterScale - 98}
          y={100}
          width={92}
          height={32}
          radius={12}
          fill={colors.surface}
          stroke={colors.api}
          lineWidth={2}
        >
          <Txt text={'node 1'} fill={colors.textPrimary} fontSize={16} fontWeight={700} />
        </Rect>
      </Layout>
      <Layout ref={joinNode2Ref} opacity={0}>
        <Rect
          x={group4Pos[0] * clusterScale}
          y={100}
          width={92}
          height={32}
          radius={12}
          fill={colors.surface}
          stroke={colors.api}
          lineWidth={2}
        >
          <Txt text={'node 2'} fill={colors.textPrimary} fontSize={16} fontWeight={700} />
        </Rect>
      </Layout>
      <Layout ref={joinNode3Ref} opacity={0}>
        <Rect
          x={group4Pos[0] * clusterScale + 98}
          y={100}
          width={92}
          height={32}
          radius={12}
          fill={colors.surface}
          stroke={colors.api}
          lineWidth={2}
        >
          <Txt text={'node 3'} fill={colors.textPrimary} fontSize={16} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={ownershipCardRef} opacity={0}>
        <Rect
          x={622}
          y={-124}
          width={520}
          height={262}
          radius={26}
          fill={colors.surface}
          stroke={colors.panelStroke}
          lineWidth={3}
          opacity={0.96}
        />
        <Txt text={'shard ownership'} x={622} y={-218} fill={colors.textPrimary} fontSize={28} fontWeight={700} />
        <Txt text={'before'} x={512} y={-184} fill={colors.warning} fontSize={18} fontWeight={700} />
        <Txt text={'after'} x={732} y={-184} fill={colors.api} fontSize={18} fontWeight={700} />
        <Rect x={622} y={-106} width={2} height={146} fill={colors.panelStroke} opacity={0.7} />

        <Txt text={'group 1 : S0 S1 S2'} x={512} y={-138} fill={colors.warning} fontSize={20} fontWeight={700} />
        <Txt text={'group 2 : S3 S4 S5'} x={512} y={-102} fill={colors.warning} fontSize={20} fontWeight={700} />
        <Txt text={'group 3 : S6 S7'} x={512} y={-66} fill={colors.warning} fontSize={20} fontWeight={700} />

        <Txt text={'group 1 : S0 S1'} x={732} y={-138} fill={colors.api} fontSize={20} fontWeight={700} />
        <Txt text={'group 2 : S3 S4'} x={732} y={-102} fill={colors.api} fontSize={20} fontWeight={700} />
        <Txt text={'group 3 : S6 S7'} x={732} y={-66} fill={colors.api} fontSize={20} fontWeight={700} />
        <Txt text={'group 4 : S2 S5'} x={732} y={-30} fill={colors.api} fontSize={20} fontWeight={700} />
      </Layout>

      <Layout ref={moveListRef} opacity={0}>
        <Txt text={'migration plan'} x={622} y={42} fill={colors.textPrimary} fontSize={28} fontWeight={700} />
        <Txt text={'1. S2 : group 1 -> group 4'} x={622} y={82} fill={colors.warning} fontSize={20} fontWeight={700} />
        <Txt text={'2. S5 : group 2 -> group 4'} x={622} y={116} fill={colors.warning} fontSize={20} fontWeight={700} />
        <Txt text={'lock shard, copy data, then switch ownership'} x={622} y={148} fill={colors.textSecondary} fontSize={18} />
      </Layout>

      <Layout ref={stageRef} opacity={0}>
        <Rect
          x={0}
          y={44}
          width={250}
          height={118}
          radius={24}
          fill={colors.surface}
          stroke={colors.warning}
          lineWidth={3}
          opacity={0.88}
        >
          <Txt text={'migration in progress'} y={-22} fill={colors.textPrimary} fontSize={22} fontWeight={700} />
          <Txt text={'two shards move into group 4'} y={6} fill={colors.textSecondary} fontSize={15} />
        </Rect>
      </Layout>

      <Layout ref={move1Ref} opacity={0}>
        <MigrationChip x={0} y={0} label={rebalanceMoves[0].shard} />
      </Layout>
      <Layout ref={move2Ref} opacity={0}>
        <MigrationChip x={0} y={0} label={rebalanceMoves[1].shard} />
      </Layout>
    </>,
  );

  yield* all(clusterRef().opacity(1, 0.8), controlRef().opacity(1, 0.7));
  yield* waitFor(0.45);
  yield* all(group4GhostRef().opacity(0.8, 0.7), joinBadgeRef().opacity(1, 0.5));
  yield* waitFor(0.18);
  yield* joinNode1Ref().opacity(1, 0.28);
  yield* waitFor(0.12);
  yield* joinNode2Ref().opacity(1, 0.28);
  yield* waitFor(0.12);
  yield* joinNode3Ref().opacity(1, 0.28);
  yield* waitFor(0.36);
  yield* all(
    joinNode1Ref().opacity(0, 0.22),
    joinNode2Ref().opacity(0, 0.22),
    joinNode3Ref().opacity(0, 0.22),
  );
  yield* waitFor(0.12);
  yield* flashTwice(group4GhostRef(), 0.95, 0.42, 0.18, 0.12, 0.12);
  yield* waitFor(0.25);

  yield* ownershipCardRef().opacity(1, 0.6);
  yield* waitFor(1.1);
  yield* moveListRef().opacity(1, 0.6);
  yield* waitFor(0.45);

  move1Ref().position(sourcePositions[0]);
  move2Ref().position(sourcePositions[1]);

  yield* all(
    move1Ref().opacity(1, 0.4),
    move2Ref().opacity(1, 0.4),
  );

  yield* stageRef().opacity(1, 0.55);
  yield* flashTwice(stageRef(), 1, 0.72, 0.14, 0.12, 0.12);
  yield* all(
    group1BeforeRef().opacity(0, 0.42),
    group1AfterRef().opacity(1, 0.42),
    group2BeforeRef().opacity(0, 0.42),
    group2AfterRef().opacity(1, 0.42),
    move1Ref().position(centerStagePositions[0], 1.35),
    move2Ref().position(centerStagePositions[1], 1.35),
  );
  yield* waitFor(0.38);

  yield* all(
    stageRef().opacity(0.08, 1.55),
    move1Ref().position(targetPositions[0], 1.55),
    move2Ref().position(targetPositions[1], 1.55),
  );

  yield* all(
    group4GhostRef().opacity(0, 0.35),
    group4AfterRef().opacity(1, 0.35),
    move1Ref().opacity(0, 0.35),
    move2Ref().opacity(0, 0.35),
  );
  yield* flashTwice(group4AfterRef(), 1, 0.7, 0.14, 0.1, 0.12);

  yield* waitFor(2.0);
});
