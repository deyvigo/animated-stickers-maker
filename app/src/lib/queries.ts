import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api, CreateRenderInput, Job, Render } from '@/lib/api';

const ACTIVE_JOB_STATUSES = new Set(['pending', 'downloading', 'processing']);
const ACTIVE_RENDER_STATUSES = new Set(['pending', 'rendering']);

/** Polls a job while it's downloading/processing, stops once ready/failed. */
export function useJob(jobId: string | undefined) {
  return useQuery({
    queryKey: ['job', jobId],
    queryFn: () => api.getJob(jobId as string),
    enabled: !!jobId,
    refetchInterval: (query) => {
      const job = query.state.data as Job | undefined;
      return job && ACTIVE_JOB_STATUSES.has(job.status) ? 1500 : false;
    },
  });
}

export function useCreateJob() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (url: string) => api.createJob(url),
    onSuccess: (job) => {
      queryClient.setQueryData(['job', job.job_id], job);
    },
  });
}

/** Polls a render while it's pending/rendering, stops once ready/failed. */
export function useRender(renderId: string | undefined) {
  return useQuery({
    queryKey: ['render', renderId],
    queryFn: () => api.getRender(renderId as string),
    enabled: !!renderId,
    refetchInterval: (query) => {
      const render = query.state.data as Render | undefined;
      return render && ACTIVE_RENDER_STATUSES.has(render.status) ? 1000 : false;
    },
  });
}

export function useCreateRender(jobId: string | undefined) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateRenderInput) => api.createRender(jobId as string, input),
    onSuccess: (render) => {
      queryClient.setQueryData(['render', render.render_id], render);
    },
  });
}
