DROP INDEX IF EXISTS public.idx_support_ticket_replies_ticket_created;
DROP TABLE IF EXISTS public.support_ticket_replies;

DROP INDEX IF EXISTS public.idx_support_tickets_store_email_created;
DROP INDEX IF EXISTS public.idx_support_tickets_conversation;
DROP INDEX IF EXISTS public.idx_support_tickets_store_status_created;
DROP INDEX IF EXISTS public.idx_support_tickets_store_number;
DROP TABLE IF EXISTS public.support_tickets;
