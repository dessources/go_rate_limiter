import { Loader, Play, Trash, StopCircle } from "lucide-react";
import { Button } from "./ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "./ui/card";
import { useEffect, useState } from "react";
import { BASE_URL } from "@/lib/utils";
import { EventSourcePolyfill } from "event-source-polyfill";

enum StressTestStatus {
  DONE,
  RUNNING,
  READY,
}

export default function StressTest() {
  const terminalStartMessage = "Waiting for test to start...";

  const [stressTestStatus, setStressTestStatus] = useState<StressTestStatus>(
    StressTestStatus.READY,
  );

  const [stressTestOutput, setStressTestOutput] =
    useState(terminalStartMessage);
  const [error, setError] = useState("");
  const [evtSource, setEvtSource] = useState<EventSource>();

  const handleStressTest = () => {
    setStressTestStatus(StressTestStatus.RUNNING);
    setStressTestOutput(terminalStartMessage);
    setError("");
    setStressTestOutput("Initializing stress test...\n");

    try {
      const evtSource = new EventSourcePolyfill(
        `${BASE_URL}/api/stress-test/stream`,
        {
          headers: {
            "X-API-KEY": "Some-random_key",
          },
        },
      );
      setEvtSource(evtSource);

      evtSource.onmessage = (e) => {
        if (e.data) {
          const parsedData = JSON.parse(e.data);
          if (parsedData.error) {
            setError(parsedData.error);
            evtSource.close();
            setStressTestStatus(StressTestStatus.DONE);
          } else {
            setStressTestOutput((prev) => prev + "\n" + parsedData.outputLine);
          }
        }
      };

      evtSource.addEventListener("error", () => {
        setError(
          "Rate limit for stress test feature reached or connection may have failed. Try again in a minute.",
        );
        setStressTestStatus(StressTestStatus.READY);
        evtSource.close();
      });

      evtSource.addEventListener("done", (e) => {
        const { data } = e as MessageEvent;
        if (data) {
          const parsedData = JSON.parse(data);
          setStressTestOutput((prev) => prev + "\n" + parsedData.outputLine);
        }
        setStressTestStatus(StressTestStatus.DONE);
        evtSource.close();
      });
    } catch (error) {
      console.log("i did this: ", error);
    }
  };
  const handleReset = () => {
    if (testIsRunning && evtSource) {
      evtSource.close();
    }
    setStressTestOutput(terminalStartMessage);
    setError("");
    setStressTestStatus(StressTestStatus.READY);
  };

  const testIsRunning = stressTestStatus == StressTestStatus.RUNNING;
  const testIsReady = stressTestStatus == StressTestStatus.READY;

  return (
    <div>
      <h2 className="mb-4 text-2xl font-semibold text-foreground">
        System Stress Test
      </h2>
      <Card>
        <CardContent className="pt-6">
          <div className="space-y-4">
            <Button
              variant={testIsRunning ? "secondary" : "destructive"}
              onClick={handleStressTest}
              disabled={testIsRunning}
              className="mr-2 "
            >
              {testIsRunning ? (
                <>
                  <Loader className="mr-2 h-4 w-4 animate-spin" />
                  Running Test...
                </>
              ) : (
                <>
                  <Play className="mr-2 h-4 w-4" />
                  Run Stress Test
                </>
              )}
            </Button>
            <Button
              variant={testIsRunning ? "destructive" : "secondary"}
              onClick={handleReset}
              disabled={testIsReady}
            >
              {testIsRunning ? (
                <>
                  <StopCircle className="mr-2 h-4 " />
                  Stop test
                </>
              ) : (
                <>
                  <Trash className="mr-2 h-4 w-4" />
                  Reset
                </>
              )}
            </Button>
            <p className="text-sm text-ghost mb-4">
              This will spin up a test server and simulate high traffic to it.
              The test takes less than a minute to complete.
            </p>
            <Card className="bg-black mt-7">
              <CardHeader>
                <CardTitle className="text-sm font-mono text-green-400">
                  Test Output
                </CardTitle>
              </CardHeader>
              <CardContent>
                {error ? (
                  <pre className="max-h-50  overflow-auto font-mono text-xs text-red-400/90">
                    {error + "\n\n\n\n\n\n\n"}
                  </pre>
                ) : (
                  <pre className="min-h-35 max-h-130  overflow-auto font-mono text-xs text-green-400/90">
                    {stressTestOutput + "\n\n\n\n\n\n\n"}
                  </pre>
                )}
              </CardContent>
            </Card>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
