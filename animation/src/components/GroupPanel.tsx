import {Rect, Txt} from '@motion-canvas/2d';

import {colors} from '../theme/colors';
import {NodeCard} from './NodeCard';
import {ShardChip} from './ShardChip';

export interface GroupPanelProps {
  x: number;
  y: number;
  groupId: string;
  nodes: string[];
  leaderId: string;
  shards: string[];
  hiddenShards?: string[];
  failedNodes?: string[];
}

const shardPalette = [
  colors.shardA,
  colors.shardB,
  colors.shardC,
  colors.shardD,
];

export function GroupPanel({
  x,
  y,
  groupId,
  nodes,
  leaderId,
  shards,
  hiddenShards = [],
  failedNodes = [],
}: GroupPanelProps) {
  const visibleShards = shards.filter(shard => !hiddenShards.includes(shard));
  const shardRows = Math.ceil(Math.max(visibleShards.length, 1) / 4);
  const panelHeight = shardRows > 1 ? 314 : 270;
  const shardBaseY = shardRows > 1 ? 86 : 96;

  return (
    <Rect
      x={x}
      y={y}
      width={410}
      height={panelHeight}
      radius={24}
      fill={colors.surface}
      stroke={colors.panelStroke}
      lineWidth={3}
      shadowBlur={14}
      shadowColor={colors.backgroundAccent}
    >
      <Txt
        x={-132}
        y={-100}
        text={groupId}
        fill={colors.textPrimary}
        fontSize={30}
        fontWeight={700}
      />
      <Txt
        x={74}
        y={-100}
        text={'raft replica group'}
        fill={colors.textSecondary}
        fontSize={16}
      />

      {nodes.map((node, index) => (
        <NodeCard
          x={-118 + index * 118}
          y={-20}
          label={node}
          leader={node === leaderId}
          failed={failedNodes.includes(node)}
        />
      ))}

      <Txt
        x={-126}
        y={56}
        text={'owned shards'}
        fill={colors.textSecondary}
        fontSize={16}
      />

      {visibleShards.map((shard, index) => (
        <ShardChip
          x={-118 + (index % 4) * 82}
          y={shardBaseY + Math.floor(index / 4) * 42}
          label={shard}
          color={shardPalette[index % shardPalette.length]}
        />
      ))}
    </Rect>
  );
}
