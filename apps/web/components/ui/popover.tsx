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
          "z-50 w-96 overflow-y-auto overscroll-contain border border-rule bg-blotter p-5 text-ink shadow-none outline-none focus-visible:ring-2 focus-visible:ring-ink",
          className
        )}
        {...props}
      />
    </PopoverPrimitive.Portal>
  );
}

export const PopoverArrow = PopoverPrimitive.Arrow;
