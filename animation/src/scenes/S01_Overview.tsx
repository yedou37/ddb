import {
  Layout,
  Line,
  Rect,
  Txt,
  makeScene2D,
} from '@motion-canvas/2d';
import {all, createRef, waitFor} from '@motion-canvas/core';

import {overviewTopology} from '../data/topology';
import {demoRequests} from '../data/requests';
import {ApiServerBox} from '../components/ApiServerBox';
import {EtcdCylinder} from '../components/EtcdCylinder';
import {GroupPanel} from '../components/GroupPanel';
import {RequestPacket} from '../components/RequestPacket';
import {colors} from '../theme/colors';

export default makeScene2D(function* (view) {
  const clientRef = createRef<Layout>();
  const etcdRef = createRef<Layout>();
  const apiRef = createRef<Layout>();
  const g1Ref = createRef<Layout>();
  const g2Ref = createRef<Layout>();
  const g3Ref = createRef<Layout>();

  const clientToApi = createRef<Line>();
  const apiToEtcd = createRef<Line>();
  const apiToG1 = createRef<Line>();
  const apiToG3 = createRef<Line>();
  const packetARef = createRef<Layout>();
  const packetBRef = createRef<Layout>();
  const routeTagARef = createRef<Layout>();
  const routeTagBRef = createRef<Layout>();

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
        text={'ShardDB Overview'}
        fill={colors.textPrimary}
        fontSize={56}
        fontWeight={700}
      />
      <Txt
        x={0}
        y={-420}
        text={'control plane, routing, and shard replica groups'}
        fill={colors.textSecondary}
        fontSize={26}
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
          <Txt text={'INSERT / SELECT'} y={18} fill={colors.textSecondary} fontSize={18} />
        </Rect>

        <RequestPacket
          x={-720}
          y={-8}
          title={demoRequests[0].label}
          detail={demoRequests[0].detail}
          accent={colors.shardA}
        />
        <RequestPacket
          x={-720}
          y={82}
          title={demoRequests[1].label}
          detail={demoRequests[1].detail}
          accent={colors.shardD}
        />
      </Layout>

      <Layout ref={etcdRef} opacity={0}>
        <EtcdCylinder x={-330} y={-220} />
      </Layout>

      <Layout ref={apiRef} opacity={0}>
        <ApiServerBox x={-160} y={-40} />
      </Layout>

      <Layout ref={g1Ref} opacity={0}>
        <GroupPanel
          x={-470}
          y={220}
          groupId={overviewTopology[0].id}
          leaderId={overviewTopology[0].leader}
          nodes={overviewTopology[0].nodes}
          shards={overviewTopology[0].shards}
        />
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
        ref={clientToApi}
        points={[
          [-610, -8],
          [-270, -40],
          [-270, 82],
        ]}
        lineWidth={6}
        stroke={colors.request}
        endArrow
        end={0}
      />

      <Line
        ref={apiToEtcd}
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
        ref={apiToG1}
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
        ref={apiToG3}
        points={[
          [-20, 10],
          [180, 120],
          [380, 160],
        ]}
        lineWidth={6}
        stroke={colors.warning}
        endArrow
        end={0}
      />

      <Layout ref={packetARef} opacity={0}>
        <RequestPacket
          x={0}
          y={0}
          title={demoRequests[0].label}
          detail={demoRequests[0].detail}
          accent={colors.shardA}
        />
      </Layout>

      <Layout ref={packetBRef} opacity={0}>
        <RequestPacket
          x={0}
          y={0}
          title={demoRequests[1].label}
          detail={demoRequests[1].detail}
          accent={colors.shardD}
        />
      </Layout>

      <Layout ref={routeTagARef} opacity={0}>
        <Rect
          x={-470}
          y={64}
          width={220}
          height={56}
          radius={14}
          fill={colors.surface}
          stroke={colors.shardA}
          lineWidth={3}
        >
          <Txt text={'id=7 -> S2 -> group 1'} fill={colors.textPrimary} fontSize={18} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={routeTagBRef} opacity={0}>
        <Rect
          x={470}
          y={64}
          width={244}
          height={56}
          radius={14}
          fill={colors.surface}
          stroke={colors.shardD}
          lineWidth={3}
        >
          <Txt text={'id=101 -> S9 -> group 3'} fill={colors.textPrimary} fontSize={18} fontWeight={700} />
        </Rect>
      </Layout>

      <Txt
        x={0}
        y={470}
        text={'same table, different keys, different shards'}
        fill={colors.textSecondary}
        fontSize={26}
        opacity={0}
      />
    </>,
  );

  yield* all(clientRef().opacity(1, 0.5), etcdRef().opacity(1, 0.6), apiRef().opacity(1, 0.6));
  yield* all(g1Ref().opacity(1, 0.5), g2Ref().opacity(1, 0.5), g3Ref().opacity(1, 0.5));

  yield* clientToApi().end(1, 0.7);
  packetARef().position([-720, -8]);
  packetBRef().position([-720, 82]);
  yield* all(packetARef().opacity(1, 0.25), packetBRef().opacity(1, 0.25));
  yield* all(packetARef().position([-520, -8], 0.6), packetBRef().position([-520, 82], 0.6));
  yield* all(packetARef().position([-160, -40], 0.7), packetBRef().position([-160, 14], 0.7));

  yield* apiToEtcd().end(1, 0.5);
  yield* all(apiToG1().end(1, 0.7), apiToG3().end(1, 0.7));
  yield* all(packetARef().position([-360, 160], 0.8), packetBRef().position([380, 160], 0.8));
  yield* all(routeTagARef().opacity(1, 0.35), routeTagBRef().opacity(1, 0.35));
  yield* all(g1Ref().scale(1.03, 0.25).to(1, 0.25), g3Ref().scale(1.03, 0.25).to(1, 0.25));
  yield* waitFor(0.6);
});
