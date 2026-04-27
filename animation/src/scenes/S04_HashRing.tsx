import {Layout, Line, Rect, Txt, makeScene2D} from '@motion-canvas/2d';
import {all, createRef, waitFor} from '@motion-canvas/core';

import {colors} from '../theme/colors';

interface TableRow {
  id: number;
  name: string;
  shard: string;
  group: string;
  accent: string;
}

const tableRows: TableRow[] = [
  {id: 1, name: 'mike', shard: 'S1', group: 'group 2', accent: colors.shardB},
  {id: 2, name: 'bob', shard: 'S2', group: 'group 3', accent: colors.shardC},
  {id: 3, name: 'liam', shard: 'S3', group: 'group 3', accent: colors.shardD},
  {id: 4, name: 'iris', shard: 'S0', group: 'group 1', accent: colors.shardA},
  {id: 5, name: 'nina', shard: 'S1', group: 'group 2', accent: colors.shardB},
  {id: 6, name: 'zoe', shard: 'S2', group: 'group 3', accent: colors.shardC},
  {id: 7, name: 'emma', shard: 'S3', group: 'group 3', accent: colors.shardD},
  {id: 8, name: 'alice', shard: 'S0', group: 'group 1', accent: colors.shardA},
];

const shardTables = [
  {id: 'S0', x: 120, y: -230, group: 'group 1', accent: colors.shardA, rows: ['id=4', 'id=8']},
  {id: 'S1', x: 120, y: -50, group: 'group 2', accent: colors.shardB, rows: ['id=1', 'id=5']},
  {id: 'S2', x: 120, y: 130, group: 'group 3', accent: colors.shardC, rows: ['id=2', 'id=6']},
  {id: 'S3', x: 120, y: 310, group: 'group 3', accent: colors.shardD, rows: ['id=3', 'id=7']},
] as const;

const groupTables = [
  {title: 'owner group 1', detail: 'holds shard S0', x: 620, y: -230, accent: colors.shardA, rows: ['id=4', 'id=8']},
  {title: 'owner group 2', detail: 'holds shard S1', x: 620, y: -10, accent: colors.shardB, rows: ['id=1', 'id=5']},
  {title: 'owner group 3', detail: 'holds shard S2 + S3', x: 620, y: 250, accent: colors.shardD, rows: ['id=2', 'id=6', 'id=3', 'id=7']},
] as const;

export default makeScene2D(function* (view) {
  const rowRefs = tableRows.map(() => createRef<Layout>());
  const shardRefs = shardTables.map(() => createRef<Layout>());
  const groupTableRefs = groupTables.map(() => createRef<Layout>());
  const groupFrameRefs = groupTables.map(() => createRef<Layout>());

  const guideARef = createRef<Line>();
  const guideBRef = createRef<Line>();
  const guideCRef = createRef<Line>();
  const guideDRef = createRef<Line>();

  const rowSourcePositions: [number, number][] = [
    [-600, -150],
    [-600, -90],
    [-600, -30],
    [-600, 30],
    [-600, 90],
    [-600, 150],
    [-600, 210],
    [-600, 270],
  ];

  const rowTargetPositions: [number, number][] = [
    [120, -52],
    [120, 128],
    [120, 308],
    [120, -232],
    [120, -24],
    [120, 156],
    [120, 336],
    [120, -204],
  ];

  const rowFinalPositions: [number, number][] = [
    [620, -12],
    [570, 248],
    [670, 248],
    [620, -232],
    [620, 16],
    [570, 276],
    [670, 276],
    [620, -204],
  ];

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
        text={'Row Sharding'}
        fill={colors.textPrimary}
        fontSize={56}
        fontWeight={700}
      />
      <Txt
        x={0}
        y={-420}
        text={'rows are split by primary key mod 4, grouped into four shards, then assigned to three owner groups'}
        fill={colors.textSecondary}
        fontSize={24}
      />

      <Rect x={-600} y={60} width={370} height={620} radius={28} fill={colors.surface} stroke={colors.panelStroke} lineWidth={3} />
      <Txt x={-680} y={-230} text={'users table'} fill={colors.textPrimary} fontSize={34} fontWeight={700} />
      <Txt x={-510} y={-230} text={'primary key -> mod 4'} fill={colors.textSecondary} fontSize={18} />
      <Rect x={-600} y={-176} width={318} height={42} radius={12} fill={colors.backgroundAccent} opacity={0.82}>
        <Txt text={'id'} x={-116} fill={colors.textSecondary} fontSize={18} fontWeight={700} />
        <Txt text={'name'} x={-8} fill={colors.textSecondary} fontSize={18} fontWeight={700} />
        <Txt text={'mod 4'} x={108} fill={colors.textSecondary} fontSize={18} fontWeight={700} />
      </Rect>

      {tableRows.map((row, index) => (
        <Rect
          x={-600}
          y={-150 + index * 60}
          width={318}
          height={42}
          radius={12}
          fill={colors.surfaceMuted}
          stroke={row.accent}
          lineWidth={2}
          opacity={0.46}
        >
          <Txt text={`${row.id}`} x={-116} fill={colors.textPrimary} fontSize={17} fontWeight={700} />
          <Txt text={row.name} x={-6} fill={colors.textPrimary} fontSize={17} />
          <Txt text={`${row.id % 4}`} x={108} fill={colors.textSecondary} fontSize={17} />
        </Rect>
      ))}

      <Txt x={120} y={-300} text={'4 shards'} fill={colors.textPrimary} fontSize={30} fontWeight={700} />
      <Txt x={642} y={-340} text={'3 owner groups'} fill={colors.textPrimary} fontSize={30} fontWeight={700} />

      <Line points={[[-190, -120], [850, -120]]} stroke={colors.panelStroke} lineWidth={2} opacity={0.2} />
      <Line points={[[-190, 100], [850, 100]]} stroke={colors.panelStroke} lineWidth={2} opacity={0.2} />
      <Line points={[[-190, 320], [850, 320]]} stroke={colors.panelStroke} lineWidth={2} opacity={0.2} />

      <Line ref={guideARef} points={[[-350, -118], [20, -230]]} stroke={colors.shardA} lineWidth={4} endArrow end={0} />
      <Line ref={guideBRef} points={[[-350, 2], [20, -50]]} stroke={colors.shardB} lineWidth={4} endArrow end={0} />
      <Line ref={guideCRef} points={[[-350, 122], [20, 130]]} stroke={colors.shardC} lineWidth={4} endArrow end={0} />
      <Line ref={guideDRef} points={[[-350, 242], [20, 310]]} stroke={colors.shardD} lineWidth={4} endArrow end={0} />

      {shardTables.map((table, index) => (
        <Layout ref={shardRefs[index]} opacity={0}>
          <Rect
            x={0}
            y={0}
            width={150}
            height={112}
            radius={18}
            fill={colors.surface}
            stroke={table.accent}
            lineWidth={3}
          >
            <Txt text={`shard ${table.id}`} y={-34} fill={colors.textPrimary} fontSize={24} fontWeight={700} />
            {table.rows.map((rowText, rowIndex) => (
              <Rect
                x={0}
                y={-2 + rowIndex * 28}
                width={110}
                height={22}
                radius={8}
                fill={colors.backgroundAccent}
                opacity={0.95}
              >
                <Txt text={rowText} fill={colors.textSecondary} fontSize={13} />
              </Rect>
            ))}
          </Rect>
        </Layout>
      ))}

      {groupTables.map((group, index) => (
        <Layout ref={groupFrameRefs[index]} opacity={0.24}>
          <Rect
            x={group.x}
            y={group.y}
            width={292}
            height={group.rows.length > 2 ? 198 : 162}
            radius={22}
            fill={colors.surface}
            stroke={colors.panelStroke}
            lineWidth={2}
          >
            <Txt text={group.title} y={-54} fill={colors.textPrimary} fontSize={24} fontWeight={700} />
            <Txt text={'replica group'} y={-26} fill={colors.textSecondary} fontSize={14} />
            <Rect x={group.x - 78} y={group.y + 2} width={62} height={20} radius={8} fill={colors.backgroundAccent} opacity={0.82}>
              <Txt text={'n1'} fill={colors.textSecondary} fontSize={12} fontWeight={700} />
            </Rect>
            <Rect x={group.x} y={group.y + 2} width={62} height={20} radius={8} fill={colors.backgroundAccent} opacity={0.82}>
              <Txt text={'n2'} fill={colors.textSecondary} fontSize={12} fontWeight={700} />
            </Rect>
            <Rect x={group.x + 78} y={group.y + 2} width={62} height={20} radius={8} fill={colors.backgroundAccent} opacity={0.82}>
              <Txt text={'n3'} fill={colors.textSecondary} fontSize={12} fontWeight={700} />
            </Rect>
            <Txt text={'owned shards'} y={36} fill={colors.textSecondary} fontSize={14} />
            <Rect x={group.x - 42} y={group.y + 64} width={64} height={24} radius={10} fill={colors.backgroundAccent} opacity={0.9}>
              <Txt text={index === 0 ? 'S0' : index === 1 ? 'S1' : 'S2'} fill={colors.textSecondary} fontSize={13} fontWeight={700} />
            </Rect>
            <Rect x={group.x + 42} y={group.y + 64} width={64} height={24} radius={10} fill={colors.backgroundAccent} opacity={0.9}>
              <Txt text={index === 2 ? 'S3' : '-'} fill={colors.textSecondary} fontSize={13} fontWeight={700} />
            </Rect>
          </Rect>
        </Layout>
      ))}

      {groupTables.map((group, index) => (
        <Layout ref={groupTableRefs[index]} opacity={0}>
          <Rect
            x={group.x}
            y={group.y}
            width={230}
            height={group.rows.length > 2 ? 124 : 92}
            radius={18}
            fill={colors.surface}
            stroke={group.accent}
            lineWidth={3}
          >
            <Txt text={group.title} y={-12} fill={colors.textPrimary} fontSize={22} fontWeight={700} />
            <Txt text={group.detail} y={16} fill={colors.textSecondary} fontSize={16} />
            {group.rows.map((rowText, rowIndex) => (
              <Rect
                x={group.x}
                y={group.y + 44 + rowIndex * 24}
                width={120}
                height={18}
                radius={8}
                fill={colors.backgroundAccent}
                opacity={0.92}
              >
                <Txt text={rowText} fill={colors.textSecondary} fontSize={12} />
              </Rect>
            ))}
          </Rect>
        </Layout>
      ))}

      {tableRows.map((row, index) => (
        <Layout ref={rowRefs[index]} opacity={0}>
          <Rect
            x={0}
            y={0}
            width={110}
            height={22}
            radius={8}
            fill={colors.surface}
            stroke={row.accent}
            lineWidth={2}
            shadowBlur={10}
            shadowColor={row.accent}
          >
            <Txt text={`id=${row.id}`} fill={colors.textPrimary} fontSize={12} fontWeight={700} />
          </Rect>
        </Layout>
      ))}
    </>,
  );

  rowRefs.forEach((ref, index) => {
    ref().position(rowSourcePositions[index]);
  });
  shardRefs.forEach((ref, index) => {
    ref().position([shardTables[index].x, shardTables[index].y]);
  });

  yield* all(
    ...rowRefs.map(ref => ref().opacity(1, 0.45)),
    guideARef().end(1, 0.8),
    guideBRef().end(1, 0.8),
    guideCRef().end(1, 0.8),
    guideDRef().end(1, 0.8),
  );
  yield* waitFor(0.3);

  yield* all(
    ...rowRefs.map((ref, index) => ref().position(rowTargetPositions[index], 1.35)),
    ...shardRefs.map(ref => ref().opacity(1, 0.55)),
  );
  yield* waitFor(0.3);

  yield* all(
    shardRefs[0]().position([620, -230], 1.45),
    shardRefs[1]().position([620, -10], 1.45),
    shardRefs[2]().position([570, 250], 1.45),
    shardRefs[3]().position([670, 250], 1.45),
    ...rowRefs.map((ref, index) => ref().position(rowFinalPositions[index], 1.45)),
  );

  yield* all(
    ...groupFrameRefs.map(ref => ref().opacity(0.9, 0.6)),
    ...groupTableRefs.map(ref => ref().opacity(1, 0.6)),
    ...shardRefs.map(ref => ref().opacity(0, 0.4)),
    ...rowRefs.map(ref => ref().opacity(0, 0.38)),
  );

  yield* waitFor(1.3);
});
