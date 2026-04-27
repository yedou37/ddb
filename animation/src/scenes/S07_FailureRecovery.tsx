import {Layout, Line, Rect, Txt, makeScene2D} from '@motion-canvas/2d';
import {all, createRef, waitFor} from '@motion-canvas/core';

import {overviewTopology} from '../data/topology';
import {demoRequests} from '../data/requests';
import {ApiServerBox} from '../components/ApiServerBox';
import {EtcdCylinder} from '../components/EtcdCylinder';
import {GroupPanel} from '../components/GroupPanel';
import {NodeCard} from '../components/NodeCard';
import {RequestPacket} from '../components/RequestPacket';
import {colors} from '../theme/colors';
import {flashTwice} from '../utils/animation';

export default makeScene2D(function* (view) {
  const clientRef = createRef<Layout>();
  const etcdRef = createRef<Layout>();
  const apiRef = createRef<Layout>();
  const g1BaseRef = createRef<Layout>();
  const g1FailureRef = createRef<Layout>();
  const g1RecoveredRef = createRef<Layout>();
  const g2Ref = createRef<Layout>();
  const g3Ref = createRef<Layout>();
  const apiToEtcdRef = createRef<Line>();
  const apiToG1Ref = createRef<Line>();
  const writePacketRef = createRef<Layout>();
  const failureBadgeRef = createRef<Layout>();
  const electionBadgeRef = createRef<Layout>();
  const recoveryBadgeRef = createRef<Layout>();

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
        text={'Failure And Recovery'}
        fill={colors.textPrimary}
        fontSize={56}
        fontWeight={700}
      />
      <Txt
        x={0}
        y={-420}
        text={'the full cluster stays visible while one node fails, leadership shifts, and the system recovers'}
        fill={colors.textSecondary}
        fontSize={22}
      />

      <Layout ref={clientRef} opacity={0}>
        <Rect
          x={-720}
          y={-92}
          width={220}
          height={90}
          radius={18}
          fill={colors.surface}
          stroke={colors.request}
          lineWidth={3}
        >
          <Txt text={'client SQL'} y={-10} fill={colors.textPrimary} fontSize={30} fontWeight={700} />
          <Txt text={'write keeps flowing'} y={18} fill={colors.textSecondary} fontSize={18} />
        </Rect>

        <RequestPacket
          x={-720}
          y={-8}
          title={demoRequests[0].label}
          detail={demoRequests[0].detail}
          accent={colors.shardA}
        />
      </Layout>

      <Layout ref={etcdRef} opacity={0}>
        <EtcdCylinder x={-330} y={-220} />
      </Layout>

      <Layout ref={apiRef} opacity={0}>
        <ApiServerBox x={-160} y={-40} />
      </Layout>

      <Layout ref={g1BaseRef} opacity={0}>
        <GroupPanel
          x={-470}
          y={220}
          groupId={overviewTopology[0].id}
          leaderId={overviewTopology[0].leader}
          nodes={overviewTopology[0].nodes}
          shards={overviewTopology[0].shards}
        />
      </Layout>

      <Layout ref={g1FailureRef} opacity={0}>
        <Rect
          x={-470}
          y={220}
          width={410}
          height={270}
          radius={24}
          fill={colors.surface}
          stroke={colors.panelStroke}
          lineWidth={3}
          shadowBlur={14}
          shadowColor={colors.backgroundAccent}
        >
          <Txt x={-132} y={-100} text={'group 1'} fill={colors.textPrimary} fontSize={30} fontWeight={700} />
          <Txt x={74} y={-100} text={'leader failure'} fill={colors.textSecondary} fontSize={16} />
          <NodeCard x={-118} y={-20} label={'group 1-node 1'} failed />
          <NodeCard x={0} y={-20} label={'group 1-node 2'} />
          <NodeCard x={118} y={-20} label={'group 1-node 3'} />
          <Txt x={-126} y={56} text={'owned shards'} fill={colors.textSecondary} fontSize={16} />
          <Rect x={-118} y={96} width={72} height={34} radius={12} fill={colors.shardA} opacity={0.22} stroke={colors.shardA} lineWidth={2}>
            <Txt text={'S0'} fill={colors.textPrimary} fontSize={16} fontWeight={700} />
          </Rect>
          <Rect x={-36} y={96} width={72} height={34} radius={12} fill={colors.shardB} opacity={0.22} stroke={colors.shardB} lineWidth={2}>
            <Txt text={'S1'} fill={colors.textPrimary} fontSize={16} fontWeight={700} />
          </Rect>
          <Rect x={46} y={96} width={72} height={34} radius={12} fill={colors.shardC} opacity={0.22} stroke={colors.shardC} lineWidth={2}>
            <Txt text={'S2'} fill={colors.textPrimary} fontSize={16} fontWeight={700} />
          </Rect>
          <Rect x={128} y={96} width={72} height={34} radius={12} fill={colors.shardD} opacity={0.22} stroke={colors.shardD} lineWidth={2}>
            <Txt text={'S3'} fill={colors.textPrimary} fontSize={16} fontWeight={700} />
          </Rect>
        </Rect>
      </Layout>

      <Layout ref={g1RecoveredRef} opacity={0}>
        <Rect
          x={-470}
          y={220}
          width={410}
          height={270}
          radius={24}
          fill={colors.surface}
          stroke={colors.panelStroke}
          lineWidth={3}
          shadowBlur={14}
          shadowColor={colors.backgroundAccent}
        >
          <Txt x={-132} y={-100} text={'group 1'} fill={colors.textPrimary} fontSize={30} fontWeight={700} />
          <Txt x={74} y={-100} text={'leadership shifted'} fill={colors.textSecondary} fontSize={16} />
          <NodeCard x={-118} y={-20} label={'group 1-node 1'} failed />
          <NodeCard x={0} y={-20} label={'group 1-node 2'} leader />
          <NodeCard x={118} y={-20} label={'group 1-node 3'} />
          <Txt x={-126} y={56} text={'owned shards'} fill={colors.textSecondary} fontSize={16} />
          <Rect x={-118} y={96} width={72} height={34} radius={12} fill={colors.shardA} opacity={0.22} stroke={colors.shardA} lineWidth={2}>
            <Txt text={'S0'} fill={colors.textPrimary} fontSize={16} fontWeight={700} />
          </Rect>
          <Rect x={-36} y={96} width={72} height={34} radius={12} fill={colors.shardB} opacity={0.22} stroke={colors.shardB} lineWidth={2}>
            <Txt text={'S1'} fill={colors.textPrimary} fontSize={16} fontWeight={700} />
          </Rect>
          <Rect x={46} y={96} width={72} height={34} radius={12} fill={colors.shardC} opacity={0.22} stroke={colors.shardC} lineWidth={2}>
            <Txt text={'S2'} fill={colors.textPrimary} fontSize={16} fontWeight={700} />
          </Rect>
          <Rect x={128} y={96} width={72} height={34} radius={12} fill={colors.shardD} opacity={0.22} stroke={colors.shardD} lineWidth={2}>
            <Txt text={'S3'} fill={colors.textPrimary} fontSize={16} fontWeight={700} />
          </Rect>
        </Rect>
      </Layout>

      <Layout ref={g2Ref} opacity={0}>
        <GroupPanel
          x={0}
          y={220}
          groupId={overviewTopology[1].id}
          leaderId={overviewTopology[1].leader}
          nodes={overviewTopology[1].nodes}
          shards={overviewTopology[1].shards}
        />
      </Layout>

      <Layout ref={g3Ref} opacity={0}>
        <GroupPanel
          x={470}
          y={220}
          groupId={overviewTopology[2].id}
          leaderId={overviewTopology[2].leader}
          nodes={overviewTopology[2].nodes}
          shards={overviewTopology[2].shards}
        />
      </Layout>

      <Line
        ref={apiToEtcdRef}
        points={[
          [-160, -96],
          [-160, -182],
          [-250, -220],
        ]}
        lineWidth={5}
        stroke={colors.etcd}
        endArrow
        end={0}
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

      <Layout ref={writePacketRef} opacity={0}>
        <RequestPacket
          x={0}
          y={0}
          title={'write request'}
          detail={"INSERT id=102, name='carol'"}
          accent={colors.success}
          width={250}
        />
      </Layout>

      <Layout ref={failureBadgeRef} opacity={0}>
        <Rect
          x={520}
          y={-162}
          width={360}
          height={64}
          radius={16}
          fill={colors.surface}
          stroke={colors.danger}
          lineWidth={3}
        >
          <Txt text={'group 1-node 1 fails'} fill={colors.textPrimary} fontSize={24} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={electionBadgeRef} opacity={0}>
        <Rect
          x={520}
          y={-72}
          width={420}
          height={120}
          radius={14}
          fill={colors.surface}
          stroke={colors.success}
          lineWidth={3}
        >
          <Txt text={'leadership shifts inside group 1'} y={-24} fill={colors.textPrimary} fontSize={24} fontWeight={700} />
          <Txt text={'group 1-node 2 becomes leader'} y={8} fill={colors.textSecondary} fontSize={20} />
          <Txt text={'the rest of the cluster stays visible'} y={38} fill={colors.textSecondary} fontSize={18} />
        </Rect>
      </Layout>

      <Layout ref={recoveryBadgeRef} opacity={0}>
        <Rect
          x={520}
          y={64}
          width={430}
          height={120}
          radius={16}
          fill={colors.surface}
          stroke={colors.success}
          lineWidth={3}
        >
          <Txt text={'writes continue after re-election'} y={-24} fill={colors.textPrimary} fontSize={24} fontWeight={700} />
          <Txt text={'later, the failed node can rejoin and catch up'} y={8} fill={colors.textSecondary} fontSize={20} />
          <Txt text={'the full topology remains the same'} y={38} fill={colors.textSecondary} fontSize={18} />
        </Rect>
      </Layout>
    </>,
  );

  yield* all(
    clientRef().opacity(1, 0.7),
    etcdRef().opacity(1, 0.8),
    apiRef().opacity(1, 0.8),
    g1BaseRef().opacity(1, 0.7),
    g2Ref().opacity(1, 0.7),
    g3Ref().opacity(1, 0.7),
  );
  yield* waitFor(0.3);

  writePacketRef().position([-720, -8]);
  yield* all(apiToEtcdRef().end(1, 0.75), apiToG1Ref().end(1, 0.95), writePacketRef().opacity(1, 0.35));
  yield* writePacketRef().position([-160, -40], 0.95);
  yield* waitFor(0.25);

  yield* all(g1BaseRef().opacity(0, 0.35), g1FailureRef().opacity(1, 0.35), failureBadgeRef().opacity(1, 0.45));
  yield* flashTwice(failureBadgeRef());
  apiToG1Ref().stroke(colors.danger);
  yield* waitFor(0.65);

  yield* all(g1FailureRef().opacity(0, 0.35), g1RecoveredRef().opacity(1, 0.35), electionBadgeRef().opacity(1, 0.45));
  apiToG1Ref().stroke(colors.success);
  yield* writePacketRef().position([-360, 160], 0.95);

  yield* recoveryBadgeRef().opacity(1, 0.45);

  yield* waitFor(1.1);
});
