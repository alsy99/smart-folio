"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { api, type Sentiment } from "@/lib/api";

export function AdvisorTooltip({ sentiment }: { sentiment: Sentiment[] }) {
  const [open, setOpen] = useState(false);
  const [chat, setChat] = useState("");
  const [replies, setReplies] = useState<{ q: string; a: string }[]>([]);
  const [chatBusy, setChatBusy] = useState(false);

  async function sendChat() {
    if (!chat.trim() || chatBusy) return;
    setChatBusy(true);
    const q = chat.trim();
    setChat("");
    try {
      const r = await api.chat(q);
      setReplies((xs) => [...xs, { q, a: r.reply }]);
    } catch {
      setReplies((xs) => [...xs, { q, a: "Advisor is unavailable. Check that the gateway is up." }]);
    } finally {
      setChatBusy(false);
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" aria-expanded={open} aria-haspopup="dialog">
          Ask the Desk
        </Button>
      </PopoverTrigger>
      <PopoverContent
        side="bottom"
        align="end"
        collisionPadding={16}
        className="w-[min(26rem,calc(100vw-2rem))]"
      >
        <h3 className="text-lg font-semibold tracking-tight">Ask the Desk</h3>
        <div aria-live="polite" className="mt-3 max-h-40 space-y-2 overflow-auto">
          {replies.length === 0 && (
            <p className="text-sm text-steel">Ask why a name is tilted, or how excess versus Nifty is scored.</p>
          )}
          {replies.map((r, i) => (
            <div key={`${r.q}-${i}`} className="min-w-0 text-sm">
              <p className="font-medium break-words">{r.q}</p>
              <p className="prose-research mt-1 text-ink/80">{r.a}</p>
            </div>
          ))}
        </div>
        <form
          className="mt-3 flex gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            sendChat();
          }}
        >
          <label htmlFor="desk-ask" className="sr-only">
            Message to the desk
          </label>
          <Input
            id="desk-ask"
            name="message"
            autoComplete="off"
            spellCheck={false}
            value={chat}
            placeholder="Why is RELIANCE tilted?…"
            onChange={(e) => setChat(e.target.value)}
          />
          <Button type="submit" disabled={chatBusy}>
            {chatBusy ? "Sending…" : "Send"}
          </Button>
        </form>
        {sentiment.length > 0 && (
          <p className="mt-3 text-sm text-steel">
            {sentiment
              .slice(0, 6)
              .map((s) => `${s.symbol} ${s.label}`)
              .join(", ")}
          </p>
        )}
      </PopoverContent>
    </Popover>
  );
}
