import {GroupTopology} from './topology';

export interface RebalanceMove {
  shard: string;
  fromGroup: string;
  toGroup: string;
}

export const rebalanceBefore: GroupTopology[] = [
  {
    id: 'group 1',
    leader: 'group 1-node 1',
    nodes: ['group 1-node 1', 'group 1-node 2', 'group 1-node 3'],
    shards: ['S0', 'S1', 'S2'],
  },
  {
    id: 'group 2',
    leader: 'group 2-node 1',
    nodes: ['group 2-node 1', 'group 2-node 2', 'group 2-node 3'],
    shards: ['S3', 'S4', 'S5'],
  },
  {
    id: 'group 3',
    leader: 'group 3-node 1',
    nodes: ['group 3-node 1', 'group 3-node 2', 'group 3-node 3'],
    shards: ['S6', 'S7'],
  },
];

export const rebalanceAfter: GroupTopology[] = [
  {
    id: 'group 1',
    leader: 'group 1-node 1',
    nodes: ['group 1-node 1', 'group 1-node 2', 'group 1-node 3'],
    shards: ['S0', 'S1'],
  },
  {
    id: 'group 2',
    leader: 'group 2-node 1',
    nodes: ['group 2-node 1', 'group 2-node 2', 'group 2-node 3'],
    shards: ['S3', 'S4'],
  },
  {
    id: 'group 3',
    leader: 'group 3-node 1',
    nodes: ['group 3-node 1', 'group 3-node 2', 'group 3-node 3'],
    shards: ['S6', 'S7'],
  },
  {
    id: 'group 4',
    leader: 'group 4-node 1',
    nodes: ['group 4-node 1', 'group 4-node 2', 'group 4-node 3'],
    shards: ['S2', 'S5'],
  },
];

export const rebalanceMoves: RebalanceMove[] = [
  {shard: 'S2', fromGroup: 'group 1', toGroup: 'group 4'},
  {shard: 'S5', fromGroup: 'group 2', toGroup: 'group 4'},
];
