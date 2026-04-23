export interface DemoRequest {
  id: string;
  key: number;
  label: string;
  detail: string;
  hashToken: string;
  shard: string;
  group: string;
}

export const demoRequests: DemoRequest[] = [
  {
    id: 'req-a',
    key: 7,
    label: 'user row A',
    detail: "id=7, name='alice'",
    hashToken: '0x18',
    shard: 'S2',
    group: 'group 1',
  },
  {
    id: 'req-b',
    key: 101,
    label: 'user row B',
    detail: "id=101, name='bob'",
    hashToken: '0xa4',
    shard: 'S9',
    group: 'group 3',
  },
];
