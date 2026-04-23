export interface HashRingSlot {
  shard: string;
  group: string;
  angleDeg: number;
}

export const hashRingSlots: HashRingSlot[] = [
  {shard: 'S0', group: 'group 1', angleDeg: -120},
  {shard: 'S1', group: 'group 1', angleDeg: -92},
  {shard: 'S2', group: 'group 1', angleDeg: -58},
  {shard: 'S3', group: 'group 1', angleDeg: -26},
  {shard: 'S4', group: 'group 2', angleDeg: 6},
  {shard: 'S5', group: 'group 2', angleDeg: 34},
  {shard: 'S6', group: 'group 2', angleDeg: 66},
  {shard: 'S7', group: 'group 2', angleDeg: 98},
  {shard: 'S8', group: 'group 3', angleDeg: 132},
  {shard: 'S9', group: 'group 3', angleDeg: 164},
  {shard: 'S10', group: 'group 3', angleDeg: 198},
  {shard: 'S11', group: 'group 3', angleDeg: 232},
];
