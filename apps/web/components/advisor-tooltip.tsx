"use client";

import { useState } from "react";
import { MessageCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Popover, PopoverArrow, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { api, type Sentiment } from "@/lib/api";

export function AdvisorTooltip({ sentiment }: { sentiment: Sentiment[] }) {
  const [open, setOpen] = useState(false);
  const [chat, setChat] = useState("");
  const [replies, setReplies] = useState<{ q: string; a: string }[]>([]);
  const [chatBusy, setChatBusy] = useState(false);

  async function sendChat() {
    if (!chat.trim()) return;
    setChatBusy(true);
    const q = chat.trim();
    setChat("");
    try {
      const r = await api.chat(q);
      setReplies((xs) => [...xs, { q, a: r.reply }]);
    } catch {
      setReplies((xs) => [...xs, { q, a: "Advisor unavailable — is the gateway up?" }]);
    } finally {
      setChatBusy(false);
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          aria-expanded={open}
          aria-label="Open advisor"
          className="shadow-lg"
        >
          <MessageCircle className="size-4" />
          Advisor
        </Button>
      </PopoverTrigger>
      <PopoverContent
        side="bottom"
        align="end"
        collisionPadding={16}
        className="w-[min(26rem,calc(100vw-2rem))]"
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <PopoverArrow className="fill-white drop-shadow-[0_-1px_0_rgba(214,211,209,1)]" width={14} height={8} />
        <h3 className="font-[family-name:var(--font-display)] text-lg tracking-tight text-stone-900">
          Advisor
        </h3>
        <div className="mt-3 max-h-40 space-y-2 overflow-auto">
          {replies.length === 0 && (
            <p className="text-sm text-stone-500">
              Ask why a name is tilted, or how we score excess vs Nifty.
            </p>
          )}
          {replies.map((r, i) => (
            <div key={i} className="text-sm">
              <p className="font-medium text-stone-800">{r.q}</p>
              <p className="text-stone-600">{r.a}</p>
            </div>
          ))}
        </div>
        <div className="mt-3 flex gap-2">
          <Input
            value={chat}
            placeholder="Message the desk…"
            onChange={(e) => setChat(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && sendChat()}
          />
          <Button onClick={sendChat} disabled={chatBusy}>
            Send
          </Button>
        </div>
        {sentiment.length > 0 && (
          <p className="mt-3 text-[11px] leading-relaxed text-stone-500">
            Sentiment rollup:{" "}
            {sentiment
              .slice(0, 6)
              .map((s) => `${s.symbol} ${s.label}`)
              .join(" · ")}
          </p>
        )}
      </PopoverContent>
    </Popover>
  );
}
