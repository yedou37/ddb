import {Layout, Line, Rect, Txt, makeScene2D} from '@motion-canvas/2d';
import {all, createRef, waitFor} from '@motion-canvas/core';

import {ApiServerBox} from '../components/ApiServerBox';
import {EtcdCylinder} from '../components/EtcdCylinder';
import {GroupPanel} from '../components/GroupPanel';
import {RequestPacket} from '../components/RequestPacket';
import {overviewTopology} from '../data/topology';
import {rebalanceAfter} from '../data/rebalance';
import {colors} from '../theme/colors';
import {flashTwice} from '../utils/animation';

export default makeScene2D(function* (view) {
  const clusterScale = 0.62;
  const group1Pos: [number, number] = [-420, 150];
  const group2Pos: [number, number] = [-420, 470];
  const group3Pos: [number, number] = [420, 150];
  const group4Pos: [number, number] = [420, 470];

  const etcdRef = createRef<Layout>();
  const apiRef = createRef<Layout>();
  const clusterRef = createRef<Layout>();
  const group4Ref = createRef<Layout>();
  const joinBadgeRef = createRef<Layout>();
  const joinPacketRef = createRef<Layout>();
  const metadataBadgeRef = createRef<Layout>();
  const routeBadgeRef = createRef<Layout>();
  const g4ToApiRef = createRef<Line>();
  const apiToEtcdRef = createRef<Line>();
  const apiToG4Ref = createRef<Line>();

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
        text={'Group Join'}
        fill={colors.textPrimary}
        fontSize={56}
        fontWeight={700}
      />
      <Txt
        x={0}
        y={-420}
        text={'a new replica group registers with the control plane, topology lands in etcd, and routers start to see it'}
        fill={colors.textSecondary}
        fontSize={22}
      />

      <Layout ref={etcdRef} opacity={0}>
        <EtcdCylinder x={-330} y={-220} />
      </Layout>

      <Layout ref={apiRef} opacity={0}>
        <ApiServerBox x={-160} y={-40} />
      </Layout>

      <Layout ref={clusterRef} opacity={0}>
        <Layout scale={clusterScale}>
          <GroupPanel
            x={group1Pos[0]}
            y={group1Pos[1]}
            groupId={overviewTopology[0].id}
            leaderId={overviewTopology[0].leader}
            nodes={overviewTopology[0].nodes}
            shards={overviewTopology[0].shards}
          />
          <GroupPanel
            x={group2Pos[0]}
            y={group2Pos[1]}
            groupId={overviewTopology[1].id}
            leaderId={overviewTopology[1].leader}
            nodes={overviewTopology[1].nodes}
            shards={overviewTopology[1].shards}
          />
          <GroupPanel
            x={group3Pos[0]}
            y={group3Pos[1]}
            groupId={overviewTopology[2].id}
            leaderId={overviewTopology[2].leader}
            nodes={overviewTopology[2].nodes}
            shards={overviewTopology[2].shards}
          />
        </Layout>
      </Layout>

      <Layout ref={group4Ref} opacity={0} scale={0.92}>
        <Layout scale={clusterScale}>
          <GroupPanel
            x={group4Pos[0]}
            y={group4Pos[1]}
            groupId={rebalanceAfter[3].id}
            leaderId={rebalanceAfter[3].leader}
            nodes={rebalanceAfter[3].nodes}
            shards={[]}
          />
        </Layout>
      </Layout>

      <Line
        ref={g4ToApiRef}
        points={[
          [430, 270],
          [220, 164],
          [36, 32],
        ]}
        lineWidth={6}
        stroke={colors.api}
        endArrow
        end={0}
      />

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
        ref={apiToG4Ref}
        points={[
          [-20, 10],
          [210, 166],
          [430, 270],
        ]}
        lineWidth={6}
        stroke={colors.success}
        endArrow
        end={0}
      />

      <Layout ref={joinBadgeRef} opacity={0}>
        <Rect
          x={530}
          y={288}
          width={260}
          height={58}
          radius={16}
          fill={colors.surface}
          stroke={colors.api}
          lineWidth={3}
        >
          <Txt text={'new group 4 joins'} fill={colors.textPrimary} fontSize={22} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={joinPacketRef} opacity={0}>
        <RequestPacket
          x={0}
          y={0}
          title={'join request'}
          detail={'group 4 asks to join the cluster'}
          accent={colors.api}
          width={254}
        />
      </Layout>

      <Layout ref={metadataBadgeRef} opacity={0}>
        <Rect
          x={-430}
          y={-114}
          width={300}
          height={66}
          radius={16}
          fill={colors.surface}
          stroke={colors.etcd}
          lineWidth={3}
        >
          <Txt text={'topology v2 stored in etcd'} fill={colors.textPrimary} fontSize={22} fontWeight={700} />
        </Rect>
      </Layout>

      <Layout ref={routeBadgeRef} opacity={0}>
        <Rect
          x={150}
          y={-104}
          width={320}
          height={66}
          radius={16}
          fill={colors.surface}
          stroke={colors.success}
          lineWidth={3}
        >
          <Txt text={'routers now see group 4'} fill={colors.textPrimary} fontSize={22} fontWeight={700} />
        </Rect>
      </Layout>
    </>,
  );

  yield* all(etcdRef().opacity(1, 0.75), apiRef().opacity(1, 0.75), clusterRef().opacity(1, 0.8));
  yield* waitFor(0.3);

  yield* all(group4Ref().opacity(1, 0.7), group4Ref().scale(1, 0.7), joinBadgeRef().opacity(1, 0.45));

  joinPacketRef().position([530, 288]);
  yield* joinPacketRef().opacity(1, 0.35);
  yield* waitFor(0.2);
  yield* g4ToApiRef().end(1, 0.95);
  yield* joinPacketRef().position([26, 12], 0.95);
  yield* waitFor(0.2);

  yield* all(apiToEtcdRef().end(1, 0.75), metadataBadgeRef().opacity(1, 0.45));
  yield* waitFor(0.25);
  yield* all(apiToG4Ref().end(1, 0.95), routeBadgeRef().opacity(1, 0.45));
  yield* flashTwice(routeBadgeRef());

  yield* waitFor(2.0);
});
