"use client";

import {
  useActionState,
  useEffect,
  useId,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { useRouter } from "next/navigation";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";
import { useToast } from "@/components/feedback/Toaster";
import {
  filePlatformTicketAction,
  type PlatformTicketActionState,
} from "@/app/(admin)/support/platform/actions";

type Priority = "low" | "medium" | "high" | "urgent";

interface FieldErrors {
  subject?: string;
  description?: string;
}

const SUBJECT_MAX = 300;
const DESCRIPTION_MIN = 20;

function validate(values: { subject: string; description: string }): FieldErrors {
  const errors: FieldErrors = {};
  const subject = values.subject.trim();
  const description = values.description.trim();
  if (!subject) {
    errors.subject = "Subject is required.";
  } else if (subject.length > SUBJECT_MAX) {
    errors.subject = `Keep the subject under ${SUBJECT_MAX} characters.`;
  }
  if (!description) {
    errors.description = "Description is required.";
  } else if (description.length < DESCRIPTION_MIN) {
    errors.description = `Add at least ${DESCRIPTION_MIN} characters so the platform team can help.`;
  }
  return errors;
}

export function PlatformTicketForm() {
  const router = useRouter();
  const { toast } = useToast();

  const [state, formAction, isPending] = useActionState<
    PlatformTicketActionState,
    FormData
  >(filePlatformTicketAction, null);

  const [subject, setSubject] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState<Priority>("medium");

  // Touched + submitAttempted gate when each field's error is shown:
  // until the user blurs a field or hits submit, errors stay quiet so
  // typing the first character doesn't immediately yell at them.
  const [touched, setTouched] = useState<{
    subject?: boolean;
    description?: boolean;
  }>({});
  const [submitAttempted, setSubmitAttempted] = useState(false);

  const subjectRef = useRef<HTMLInputElement>(null);
  const descriptionRef = useRef<HTMLTextAreaElement>(null);

  const errors = validate({ subject, description });
  const subjectError =
    (touched.subject || submitAttempted) && errors.subject ? errors.subject : null;
  const descriptionError =
    (touched.description || submitAttempted) && errors.description
      ? errors.description
      : null;

  // Stable IDs so aria-describedby points at the right element on each
  // render. useId is React's hook for that.
  const subjectErrorId = useId();
  const descriptionErrorId = useId();

  // Track the last action state we've already toasted/handled so the
  // useEffect doesn't fire repeatedly across re-renders.
  const lastHandledRef = useRef<PlatformTicketActionState>(null);
  useEffect(() => {
    if (state === lastHandledRef.current) return;
    lastHandledRef.current = state;
    if (!state) return;

    if ("success" in state) {
      toast.success(
        `Ticket ${state.success.ticketNumber} filed`,
        "The Tesserix platform team will follow up by email.",
      );
      // Reset the form so the merchant can file another without
      // stale values, and refresh the RSC tree so the list below
      // re-fetches and shows the new ticket.
      setSubject("");
      setDescription("");
      setPriority("medium");
      setTouched({});
      setSubmitAttempted(false);
      router.refresh();
    } else if ("error" in state) {
      toast.error("Couldn't file ticket", state.error);
    }
  }, [state, toast, router]);

  function handleSubmit(e: FormEvent<HTMLFormElement>) {
    setSubmitAttempted(true);
    if (errors.subject || errors.description) {
      e.preventDefault();
      // Focus the first invalid field for keyboard users.
      if (errors.subject) {
        subjectRef.current?.focus();
      } else if (errors.description) {
        descriptionRef.current?.focus();
      }
    }
    // If valid, let the form fall through to the action.
  }

  return (
    <form
      action={formAction}
      onSubmit={handleSubmit}
      noValidate
      className="space-y-6"
    >
      <div className="space-y-2">
        <label
          htmlFor="subject"
          className="block text-sm font-medium text-foreground"
        >
          Subject <span className="text-[color:var(--signal)]">*</span>
        </label>
        <input
          ref={subjectRef}
          id="subject"
          name="subject"
          type="text"
          value={subject}
          onChange={(e) => setSubject(e.target.value)}
          onBlur={() => setTouched((t) => ({ ...t, subject: true }))}
          aria-invalid={subjectError ? true : undefined}
          aria-describedby={subjectError ? subjectErrorId : undefined}
          className={
            "h-11 w-full rounded-md border bg-background px-4 text-sm text-foreground placeholder:text-foreground-tertiary focus:outline-none focus:ring-1 " +
            (subjectError
              ? "border-[color:var(--signal)] focus:border-[color:var(--signal)] focus:ring-[color:var(--signal)]"
              : "border-border focus:border-[color:var(--moss-700)] focus:ring-[color:var(--moss-700)]")
          }
          placeholder="Brief summary of the issue"
        />
        {subjectError && (
          <p
            id={subjectErrorId}
            role="alert"
            className="text-xs text-[color:var(--signal)]"
          >
            {subjectError}
          </p>
        )}
      </div>

      <div className="space-y-2">
        <label
          htmlFor="description"
          className="block text-sm font-medium text-foreground"
        >
          Description <span className="text-[color:var(--signal)]">*</span>
        </label>
        <textarea
          ref={descriptionRef}
          id="description"
          name="description"
          rows={6}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          onBlur={() => setTouched((t) => ({ ...t, description: true }))}
          aria-invalid={descriptionError ? true : undefined}
          aria-describedby={descriptionError ? descriptionErrorId : undefined}
          className={
            "w-full rounded-md border bg-background px-4 py-3 text-sm text-foreground placeholder:text-foreground-tertiary focus:outline-none focus:ring-1 " +
            (descriptionError
              ? "border-[color:var(--signal)] focus:border-[color:var(--signal)] focus:ring-[color:var(--signal)]"
              : "border-border focus:border-[color:var(--moss-700)] focus:ring-[color:var(--moss-700)]")
          }
          placeholder="Tell us what happened, what you expected, and any relevant URLs or order numbers."
        />
        {descriptionError ? (
          <p
            id={descriptionErrorId}
            role="alert"
            className="text-xs text-[color:var(--signal)]"
          >
            {descriptionError}
          </p>
        ) : (
          <p className="text-xs text-foreground-tertiary">
            This goes to the Tesserix platform team — use it for billing,
            subdomain, or platform-level issues. Customer-facing tickets stay
            in <a className="underline" href="/support/tickets">Support → Tickets</a>.
          </p>
        )}
      </div>

      <div className="space-y-2">
        <label
          htmlFor="priority"
          className="block text-sm font-medium text-foreground"
        >
          Priority
        </label>
        <input type="hidden" name="priority" value={priority} />
        <Select
          value={priority}
          onValueChange={(value) => setPriority(value as Priority)}
        >
          <SelectTrigger id="priority" className="w-full sm:max-w-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="low">Low</SelectItem>
            <SelectItem value="medium">Medium</SelectItem>
            <SelectItem value="high">High</SelectItem>
            <SelectItem value="urgent">Urgent — service is down</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="flex items-center gap-4 pt-2">
        <button
          type="submit"
          disabled={isPending}
          className="inline-flex h-11 items-center justify-center rounded-md bg-primary px-6 text-sm font-medium text-primary-foreground hover:bg-primary-hover disabled:opacity-50"
        >
          {isPending ? "Filing…" : "File ticket"}
        </button>
      </div>
    </form>
  );
}
