import {Layout, Rect, Txt, makeScene2D} from '@motion-canvas/2d';
import {all, createRef, waitFor} from '@motion-canvas/core';

import {GroupPanel} from '../components/GroupPanel';
import {MigrationChip} from '../components/MigrationChip';
import {rebalanceAfter, rebalanceBefore, rebalanceMoves} from '../data/rebalance';
import {colors} from '../theme/colors';

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
  const moveListRef = createRef<Layout>();
  const stageRef = createRef<Layout>();
  const joinBadgeRef = createRef<Layout>();
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

      <Layout ref={moveListRef} opacity={0}>
        <Txt text={'migration plan'} x={622} y={-220} fill={colors.textPrimary} fontSize={28} fontWeight={700} />
        <Txt text={'S2 : group 1 -> group 4'} x={622} y={-180} fill={colors.warning} fontSize={20} fontWeight={700} />
        <Txt text={'S5 : group 2 -> group 4'} x={622} y={-146} fill={colors.warning} fontSize={20} fontWeight={700} />
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

  yield* all(clusterRef().opacity(1, 0.55), controlRef().opacity(1, 0.5));
  yield* all(group4GhostRef().opacity(0.8, 0.5), joinBadgeRef().opacity(1, 0.35));
  yield* moveListRef().opacity(1, 0.45);

  move1Ref().position(sourcePositions[0]);
  move2Ref().position(sourcePositions[1]);

  yield* all(
    move1Ref().opacity(1, 0.2),
    move2Ref().opacity(1, 0.2),
  );

  yield* stageRef().opacity(1, 0.3);
  yield* all(
    group1BeforeRef().opacity(0, 0.25),
    group1AfterRef().opacity(1, 0.25),
    group2BeforeRef().opacity(0, 0.25),
    group2AfterRef().opacity(1, 0.25),
    move1Ref().position(centerStagePositions[0], 0.9),
    move2Ref().position(centerStagePositions[1], 0.9),
  );

  yield* all(
    stageRef().opacity(0.08, 1.1),
    move1Ref().position(targetPositions[0], 1.1),
    move2Ref().position(targetPositions[1], 1.1),
  );

  yield* all(
    group4GhostRef().opacity(0, 0.2),
    group4AfterRef().opacity(1, 0.2),
    move1Ref().opacity(0, 0.2),
    move2Ref().opacity(0, 0.2),
  );

  yield* waitFor(0.15);
});
