export interface GroupTopology {
  id: string;
  leader: string;
  nodes: string[];
  shards: string[];
}

export const overviewTopology: GroupTopology[] = [
  {
    id: 'group 1',
    leader: 'group 1-node 1',
    nodes: ['group 1-node 1', 'group 1-node 2', 'group 1-node 3'],
    shards: ['S0', 'S1', 'S2', 'S3'],
  },
  {
    id: 'group 2',
    leader: 'group 2-node 1',
    nodes: ['group 2-node 1', 'group 2-node 2', 'group 2-node 3'],
    shards: ['S4', 'S5', 'S6', 'S7'],
  },
  {
    id: 'group 3',
    leader: 'group 3-node 1',
    nodes: ['group 3-node 1', 'group 3-node 2', 'group 3-node 3'],
    shards: ['S8', 'S9', 'S10', 'S11'],
  },
];
