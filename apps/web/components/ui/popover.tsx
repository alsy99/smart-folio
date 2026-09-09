"use client";

import * as React from "react";
import * as PopoverPrimitive from "@radix-ui/react-popover";
import { cn } from "@/lib/utils";

export const Popover = PopoverPrimitive.Root;
export const PopoverTrigger = PopoverPrimitive.Trigger;
export const PopoverAnchor = PopoverPrimitive.Anchor;

export function PopoverContent({
  className,
  align = "end",
  sideOffset = 10,
  ...props
}: React.ComponentProps<typeof PopoverPrimitive.Content>) {
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Content
        align={align}
        sideOffset={sideOffset}
        className={cn(
          "z-50 w-96 rounded-2xl border border-stone-200/90 bg-white p-5 text-stone-900 shadow-[0_18px_50px_-20px_rgba(28,25,23,0.45)] outline-none",
          className
        )}
        {...props}
      />
    </PopoverPrimitive.Portal>
  );
}

export const PopoverArrow = PopoverPrimitive.Arrow;
