import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createTicketsApi, type CreateTicketBody } from "@repo/mobile-shared/api/tickets";
import { useApiClient } from "@/lib/api-client";

function useTicketMutation<TVars>(
  run: (api: ReturnType<typeof createTicketsApi>, vars: TVars) => Promise<unknown>,
) {
  const client = useApiClient();
  const api = createTicketsApi(client);
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (vars: TVars) => run(api, vars),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["tickets"] });
    },
  });
}

export function useCreateTicket() {
  return useTicketMutation<CreateTicketBody>((api, body) => api.create(body));
}

export function useReplyTicket() {
  return useTicketMutation<{ id: string; content: string }>((api, { id, content }) =>
    api.reply(id, content),
  );
}

export function useUpdateTicketStatus() {
  return useTicketMutation<{ id: string; status: string }>((api, { id, status }) =>
    api.updateStatus(id, status),
  );
}
