"use client";

import * as React from "react";
import { Progress as ProgressPrimitive } from "radix-ui";

import { cn } from "@/lib/utils";

function Progress({
  className,
  value,
  ...props
}: React.ComponentProps<typeof ProgressPrimitive.Root>) {
  let percent: number = 0;
  if (!value) percent = 0;
  else percent = value;

  const getProgressColor = (percent: number) => {
    if (percent < 50) return "bg-green-500";
    if (percent < 75) return "bg-yellow-500";
    return "bg-red-500";
  };

  const bgColor = getProgressColor(percent);
  return (
    <ProgressPrimitive.Root
      data-slot="progress"
      className={cn(
        `bg-muted h-1.5 rounded-full relative flex items-center overflow-x-hidden `,
        className,
      )}
      {...props}
    >
      <ProgressPrimitive.Indicator
        data-slot="progress-indicator"
        className={`bg-muted size-full flex-1 transition-all overflow-hidden ${bgColor}`}
        style={
          percent < 50
            ? {
                transform: `translateX(-${100 - percent}%)`,
                background: "#22c55e",
              }
            : {
                transform: `translateX(-${100 - percent}%)`,
              }
        }
      />
    </ProgressPrimitive.Root>
  );
}

export { Progress };
