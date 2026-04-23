import {Circle, Layout, Line, Rect, Txt, makeScene2D} from '@motion-canvas/2d';
import {all, createRef, waitFor} from '@motion-canvas/core';

import {ApiServerBox} from '../components/ApiServerBox';
import {EtcdCylinder} from '../components/EtcdCylinder';
import {GroupPanel} from '../components/GroupPanel';
import {NodeCard} from '../components/NodeCard';
import {overviewTopology} from '../data/topology';
import {colors} from '../theme/colors';

export default makeScene2D(function* (view) {
  const healthyReturnIndexes = [3, 4, 5, 6, 7, 8];
  const logicalBeforeRef = createRef<Layout>();
  const geoShellRef = createRef<Layout>();
  const siteAFailedRef = createRef<Layout>();
  const logicalAfterRef = createRef<Layout>();
  const badgeRef = createRef<Layout>();
  const geoBadgeRef = createRef<Layout>();
  const failBadgeRef = createRef<Layout>();
  const surviveBadgeRef = createRef<Layout>();
  const finalBadgeRef = createRef<Layout>();
  const apiToG1Ref = createRef<Line>();
  const apiToG3Ref = createRef<Line>();
  const nodeFlows = [
    {label: 'group 1-node 1', source: [-588, 200] as [number, number], target: [-520, -86] as [number, number], leader: true},
    {label: 'group 2-node 1', source: [-118, 200] as [number, number], target: [-520, 12] as [number, number], leader: true},
    {label: 'group 3-node 1', source: [352, 200] as [number, number], target: [-520, 110] as [number, number], leader: true},
    {label: 'group 1-node 2', source: [-470, 200] as [number, number], target: [0, -86] as [number, number], leader: false},
    {label: 'group 2-node 2', source: [0, 200] as [number, number], target: [0, 12] as [number, number], leader: false},
    {label: 'group 3-node 2', source: [470, 200] as [number, number], target: [0, 110] as [number, number], leader: false},
    {label: 'group 1-node 3', source: [-352, 200] as [number, number], target: [520, -86] as [number, number], leader: false},
    {label: 'group 2-node 3', source: [118, 200] as [number, number], target: [520, 12] as [number, number], leader: false},
    {label: 'group 3-node 3', source: [588, 200] as [number, number], target: [520, 110] as [number, number], leader: false},
  ];
  const movingNodeRefs = nodeFlows.map(() => createRef<Layout>());

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
        text={'Real-World Disaster Recovery'}
        fill={colors.textPrimary}
        fontSize={56}
        fontWeight={700}
      />
      <Txt
        x={0}
        y={-420}
        text={'spread replicas across physical locations, lose one site, then confirm each group still has quorum'}
        fill={colors.textSecondary}
        fontSize={22}
      />

      <Layout ref={logicalBeforeRef} opacity={0}>
        <EtcdCylinder x={-330} y={-220} />
        <ApiServerBox x={-160} y={-40} />

        <GroupPanel
          x={-470}
          y={220}
          groupId={overviewTopology[0].id}
          leaderId={overviewTopology[0].leader}
          nodes={overviewTopology[0].nodes}
          shards={overviewTopology[0].shards}
        />
        <GroupPanel
          x={0}
          y={220}
          groupId={overviewTopology[1].id}
          leaderId={overviewTopology[1].leader}
          nodes={overviewTopology[1].nodes}
          shards={overviewTopology[1].shards}
        />
        <GroupPanel
          x={470}
          y={220}
          groupId={overviewTopology[2].id}
          leaderId={overviewTopology[2].leader}
          nodes={overviewTopology[2].nodes}
          shards={overviewTopology[2].shards}
        />
      </Layout>

      <Layout ref={badgeRef} opacity={0}>
        <Rect
          x={560}
          y={-184}
          width={390}
          height={72}
          radius={18}
          fill={colors.surface}
          stroke={colors.api}
          lineWidth={3}
        >
          <Txt text={'same logical cluster, different physical sites'} fill={colors.textPrimary} fontSize={22} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={geoBadgeRef} opacity={0}>
        <Rect
          x={0}
          y={-304}
          width={560}
          height={64}
          radius={16}
          fill={colors.surface}
          stroke={colors.api}
          lineWidth={3}
        >
          <Txt text={'logical replicas spread into three geographic failure domains'} fill={colors.textPrimary} fontSize={22} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={geoShellRef} opacity={0}>
        <Rect x={-520} y={20} width={360} height={510} radius={30} fill={colors.surface} stroke={colors.panelStroke} lineWidth={3}>
          <Txt text={'site A'} y={-202} fill={colors.textPrimary} fontSize={34} fontWeight={700} />
          <Txt text={'region west / room alpha'} y={-168} fill={colors.textSecondary} fontSize={18} />
        </Rect>
        <Rect x={0} y={20} width={360} height={510} radius={30} fill={colors.surface} stroke={colors.panelStroke} lineWidth={3}>
          <Txt text={'site B'} y={-202} fill={colors.textPrimary} fontSize={34} fontWeight={700} />
          <Txt text={'region central / room beta'} y={-168} fill={colors.textSecondary} fontSize={18} />
        </Rect>
        <Rect x={520} y={20} width={360} height={510} radius={30} fill={colors.surface} stroke={colors.panelStroke} lineWidth={3}>
          <Txt text={'site C'} y={-202} fill={colors.textPrimary} fontSize={34} fontWeight={700} />
          <Txt text={'region east / room gamma'} y={-168} fill={colors.textSecondary} fontSize={18} />
        </Rect>

        <Txt x={0} y={-346} text={'global deployment'} fill={colors.textPrimary} fontSize={20} fontWeight={700} />
        <Circle
          x={0}
          y={-250}
          width={176}
          height={176}
          fill={colors.backgroundAccent}
          stroke={colors.api}
          lineWidth={4}
          opacity={0.98}
        />
        <Circle
          x={0}
          y={-250}
          width={176}
          height={176}
          stroke={colors.api}
          lineWidth={2}
          scaleY={0.36}
          opacity={0.58}
        />
        <Circle
          x={0}
          y={-250}
          width={176}
          height={176}
          stroke={colors.api}
          lineWidth={2}
          scaleY={0.7}
          opacity={0.46}
        />
        <Circle
          x={0}
          y={-250}
          width={176}
          height={176}
          stroke={colors.api}
          lineWidth={2}
          scaleX={0.34}
          opacity={0.58}
        />
        <Circle
          x={0}
          y={-250}
          width={176}
          height={176}
          stroke={colors.api}
          lineWidth={2}
          scaleX={0.68}
          opacity={0.42}
        />
        <Line points={[[-88, -250], [88, -250]]} stroke={colors.api} lineWidth={2} opacity={0.32} />
        <Line points={[[0, -338], [0, -162]]} stroke={colors.api} lineWidth={2} opacity={0.25} />
        <Circle x={-56} y={-272} width={12} height={12} fill={colors.shardA} shadowBlur={8} shadowColor={colors.shardA} />
        <Circle x={8} y={-236} width={12} height={12} fill={colors.shardB} shadowBlur={8} shadowColor={colors.shardB} />
        <Circle x={62} y={-266} width={12} height={12} fill={colors.shardD} shadowBlur={8} shadowColor={colors.shardD} />
        <Txt x={-94} y={-306} text={'west'} fill={colors.shardA} fontSize={14} />
        <Txt x={4} y={-208} text={'central'} fill={colors.shardB} fontSize={14} />
        <Txt x={102} y={-294} text={'east'} fill={colors.shardD} fontSize={14} />
        <Line points={[[-56, -272], [-520, -235]]} stroke={colors.shardA} lineWidth={3} endArrow opacity={0.9} />
        <Line points={[[8, -236], [0, -235]]} stroke={colors.shardB} lineWidth={3} endArrow opacity={0.9} />
        <Line points={[[62, -266], [520, -235]]} stroke={colors.shardD} lineWidth={3} endArrow opacity={0.9} />
      </Layout>

      {nodeFlows.map((flow, index) => (
        <Layout ref={movingNodeRefs[index]} opacity={0}>
          <NodeCard x={0} y={0} label={flow.label} leader={flow.leader} />
        </Layout>
      ))}

      <Layout ref={siteAFailedRef} opacity={0}>
        <Rect x={-520} y={20} width={360} height={510} radius={30} fill={'#35181a'} stroke={colors.danger} lineWidth={4} opacity={0.94} />
        <Txt x={-520} y={-202} text={'site A'} fill={colors.textPrimary} fontSize={34} fontWeight={700} />
        <Txt x={-520} y={-168} text={'facility offline'} fill={colors.danger} fontSize={18} />
        <Rect x={-610} y={188} width={84} height={34} radius={12} fill={'#6a230f'} stroke={colors.warning} lineWidth={2}>
          <Txt text={'fire'} fill={colors.textPrimary} fontSize={16} fontWeight={700} />
        </Rect>
        <Rect x={-520} y={188} width={112} height={34} radius={12} fill={'#4a203a'} stroke={colors.danger} lineWidth={2}>
          <Txt text={'power loss'} fill={colors.textPrimary} fontSize={16} fontWeight={700} />
        </Rect>
        <Rect x={-404} y={188} width={118} height={34} radius={12} fill={'#243449'} stroke={colors.request} lineWidth={2}>
          <Txt text={'network cut'} fill={colors.textPrimary} fontSize={16} fontWeight={700} />
        </Rect>
        <NodeCard x={-520} y={-86} label={'group 1-node 1'} failed />
        <NodeCard x={-520} y={12} label={'group 2-node 1'} failed />
        <NodeCard x={-520} y={110} label={'group 3-node 1'} failed />
      </Layout>

      <Layout ref={failBadgeRef} opacity={0}>
        <Rect
          x={0}
          y={344}
          width={420}
          height={64}
          radius={16}
          fill={colors.surface}
          stroke={colors.danger}
          lineWidth={3}
        >
          <Txt text={'all machines in site A fail at once'} fill={colors.textPrimary} fontSize={22} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={surviveBadgeRef} opacity={0}>
        <Rect
          x={0}
          y={418}
          width={520}
          height={64}
          radius={16}
          fill={colors.surface}
          stroke={colors.success}
          lineWidth={3}
        >
          <Txt text={'each group still has 2 of 3 replicas alive'} fill={colors.textPrimary} fontSize={22} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={logicalAfterRef} opacity={0}>
        <EtcdCylinder x={-330} y={-220} />
        <ApiServerBox x={-160} y={-40} />

        <GroupPanel
          x={-470}
          y={220}
          groupId={overviewTopology[0].id}
          leaderId={'group 1-node 2'}
          nodes={overviewTopology[0].nodes}
          shards={overviewTopology[0].shards}
          failedNodes={['group 1-node 1']}
        />
        <GroupPanel
          x={0}
          y={220}
          groupId={overviewTopology[1].id}
          leaderId={'group 2-node 2'}
          nodes={overviewTopology[1].nodes}
          shards={overviewTopology[1].shards}
          failedNodes={['group 2-node 1']}
        />
        <GroupPanel
          x={470}
          y={220}
          groupId={overviewTopology[2].id}
          leaderId={'group 3-node 2'}
          nodes={overviewTopology[2].nodes}
          shards={overviewTopology[2].shards}
          failedNodes={['group 3-node 1']}
        />

        <Line
          ref={apiToG1Ref}
          points={[
            [-140, 10],
            [-250, 120],
            [-360, 160],
          ]}
          lineWidth={6}
          stroke={colors.success}
          endArrow
          end={0}
        />
        <Line
          ref={apiToG3Ref}
          points={[
            [-20, 10],
            [180, 120],
            [380, 160],
          ]}
          lineWidth={6}
          stroke={colors.success}
          endArrow
          end={0}
        />
      </Layout>

      <Layout ref={finalBadgeRef} opacity={0}>
        <Rect
          x={0}
          y={-184}
          width={560}
          height={72}
          radius={18}
          fill={colors.surface}
          stroke={colors.success}
          lineWidth={3}
        >
          <Txt text={'one node down per group, yet every group still works'} fill={colors.textPrimary} fontSize={22} fontWeight={700} />
        </Rect>
      </Layout>
    </>,
  );

  yield* all(logicalBeforeRef().opacity(1, 0.55), badgeRef().opacity(1, 0.35));
  yield* waitFor(0.55);

  nodeFlows.forEach((flow, index) => {
    movingNodeRefs[index]().position(flow.source);
  });
  yield* all(
    geoShellRef().opacity(1, 0.45),
    geoBadgeRef().opacity(1, 0.3),
    ...movingNodeRefs.map(ref => ref().opacity(1, 0.2)),
  );
  yield* all(
    logicalBeforeRef().opacity(0.18, 0.9),
    badgeRef().opacity(0, 0.3),
    ...nodeFlows.map((flow, index) => movingNodeRefs[index]().position(flow.target, 0.9)),
  );
  yield* logicalBeforeRef().opacity(0, 0.25);
  yield* waitFor(0.4);

  yield* all(
    movingNodeRefs[0]().opacity(0.08, 0.25),
    movingNodeRefs[1]().opacity(0.08, 0.25),
    movingNodeRefs[2]().opacity(0.08, 0.25),
    siteAFailedRef().opacity(1, 0.3),
    failBadgeRef().opacity(1, 0.3),
  );
  yield* surviveBadgeRef().opacity(1, 0.35);
  yield* waitFor(0.7);

  yield* all(
    geoShellRef().opacity(0.18, 0.35),
    geoBadgeRef().opacity(0, 0.25),
    failBadgeRef().opacity(0, 0.25),
    surviveBadgeRef().opacity(0, 0.25),
    siteAFailedRef().opacity(0.15, 0.35),
    logicalAfterRef().opacity(0.28, 0.35),
    ...healthyReturnIndexes.map(index =>
      movingNodeRefs[index]().position(nodeFlows[index].source, 0.8),
    ),
  );
  yield* all(
    logicalAfterRef().opacity(1, 0.35),
    finalBadgeRef().opacity(1, 0.35),
    ...healthyReturnIndexes.map(index => movingNodeRefs[index]().opacity(0, 0.22)),
    movingNodeRefs[0]().opacity(0, 0.22),
    movingNodeRefs[1]().opacity(0, 0.22),
    movingNodeRefs[2]().opacity(0, 0.22),
    geoShellRef().opacity(0, 0.25),
    siteAFailedRef().opacity(0, 0.25),
  );
  yield* all(apiToG1Ref().end(1, 0.55), apiToG3Ref().end(1, 0.55));
  yield* waitFor(0.8);
});
