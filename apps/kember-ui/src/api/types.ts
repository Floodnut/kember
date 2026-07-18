export interface ItemsResponse<T> {
  items: T[];
}

export interface NamespaceView {
  cluster: string;
  name: string;
}

export interface ConditionView {
  type: string;
  status: string;
  reason: string | null;
  message: string | null;
  lastTransitionTime: string | null;
}

export interface WorkerPoolCapacityView {
  desired: number;
  starting: number;
  ready: number;
  leased: number;
  terminating: number;
}

export interface WorkerPoolView {
  cluster: string;
  namespace: string;
  name: string;
  generation: number;
  executionMode: string | null;
  lifecycleProfile: string | null;
  capacity: WorkerPoolCapacityView;
  conditions: ConditionView[];
}

export interface TaskRunView {
  cluster: string;
  namespace: string;
  name: string;
  createdAt: string | null;
  workerPool: string;
  phase: string | null;
  assignedWorker: string | null;
  dispatchedAt: string | null;
  completedAt: string | null;
  conditions: ConditionView[];
  queueWaitSeconds: number | null;
  activeDurationSeconds: number | null;
}

export interface ApiErrorResponse {
  error: {
    code: string;
    message: string;
  };
}
