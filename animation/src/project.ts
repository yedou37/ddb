import {makeProject} from '@motion-canvas/core';

import overview from './scenes/S01_Overview?scene';
import sharding from './scenes/S03_Sharding?scene';
import hashRing from './scenes/S04_HashRing?scene';
import rebalanceFlow from './scenes/S06_RebalanceFlow?scene';
import failureRecovery from './scenes/S07_FailureRecovery?scene';
import realWorldDisasterRecovery from './scenes/S08_RealWorldDisasterRecovery?scene';

export default makeProject({
  scenes: [overview, sharding, hashRing, rebalanceFlow, failureRecovery, realWorldDisasterRecovery],
});
