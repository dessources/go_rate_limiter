import { Progress } from "@/components/ui/progress";
import { Card, CardHeader, CardTitle, CardContent } from "./ui/card";

import { Dispatch, SetStateAction, useEffect, useState } from "react";
import { BASE_URL } from "@/lib/utils";

import type { Metrics } from "@/app/page";

export interface LiveMetricsProps {
  metrics: Metrics;
  setMetrics: Dispatch<SetStateAction<Metrics>>;
}

export default function LiveMetrics({ metrics, setMetrics }: LiveMetricsProps) {
  useEffect(() => {
    const evtSource = new EventSource(`${BASE_URL}/api/metrics/stream`);

    evtSource.onmessage = ({ isTrusted, data }) => {
      if (isTrusted && data) {
        const parsedData: Metrics = JSON.parse(data);
        if (parsedData) setMetrics({ ...parsedData });
      }
    };

    evtSource.onerror = (err) => {
      process.env.NODE_ENV != "production" && console.error("SSE error:", err);
      evtSource.close();
    };

    return () => {
      evtSource.close();
    };
  }, [setMetrics]);

  let rateLoadPercent =
    (metrics.globalTokensUsed / metrics.globalTokenBucketCap) * 100;

  return (
    <div>
      <h2 className="mb-4 text-2xl font-semibold text-foreground">
        Live Metrics
      </h2>
      <div className="grid gap-4 md:grid-cols-3">
        {/* Global Limiter Card */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base text-center">
              Global Rate Limit
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <Progress
              value={rateLoadPercent}
              className={"[&>div]:transition-colors [&>div]:duration-500 "}
            />
            <p className="mt-10 text-center text-sm text-muted-foreground">
              {metrics.globalTokensUsed} requests /{" "}
              {metrics.globalTokenBucketCap} max
            </p>
          </CardContent>
        </Card>

        {/* Active Users Card */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base text-center">
              Active Users
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col items-center justify-center">
            <p className="text-4xl font-bold text-primary">
              {metrics.activeUsers}
            </p>
            <p className="mt-2 text-sm text-muted-foreground">
              Users in the last 30 minutes
            </p>
          </CardContent>
        </Card>

        {/* Total URLs Card */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base text-center">
              Total Active URLs
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col items-center justify-center">
            <p className="text-4xl font-bold text-primary">
              {metrics.currentUrlCount.toLocaleString()}
            </p>
            <p className="mt-2 text-sm text-muted-foreground">
              URLs created in the last hour
            </p>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
