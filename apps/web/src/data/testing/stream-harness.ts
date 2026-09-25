import type { ErrorFrame, EventBatch, Hello, Resync } from "@marshal/protocol";
import {
  createEventStream,
  type EventStreamOptions,
  type StreamDetail,
  type StreamState,
} from "../event-stream";
import { fakeSockets } from "./fake-web-socket";
import { golden } from "./golden";

/** What `random` returns for no jitter: the middle of its range. */
const NO_JITTER = 0.5;
export const STREAM_URL = "ws://localhost:3210/v1/events";
export const TOKEN = "stream-token-not-real";
export const batchFrame = golden<EventBatch>("event-batch");
export const resyncFrame = golden<Resync>("resync");
export const errorFrame = golden<ErrorFrame>("error-frame");
export const helloFrame = golden<Hello>("hello");
export const EPOCH_A = batchFrame.epoch;

export function setup(extra: Partial<EventStreamOptions> = {}) {
  const sockets = fakeSockets();
  const batches: { seqs: number[]; epoch: string }[] = [];
  const resyncs: Resync[] = [];
  const states: [StreamState, StreamDetail][] = [];
  const stream = createEventStream({
    url: STREAM_URL,
    getToken: () => TOKEN,
    WebSocketImpl: sockets.Impl,
    random: () => NO_JITTER,
    onBatch: (events, epoch) => batches.push({ seqs: events.map((e) => e.seq), epoch }),
    onResync: (frame) => resyncs.push(frame),
    onState: (state, detail) => states.push([state, detail]),
    ...extra,
  });
  /** Starts, accepts the connection, and puts the client at epoch A. */
  const open = () => {
    stream.start();
    sockets.last().accept();
    sockets.last().push({ ...resyncFrame, epoch: EPOCH_A, seq: 40 });
    return sockets.last();
  };
  return { stream, sockets, batches, resyncs, states, open };
}

export const events = (epoch: string, ...seqs: number[]): EventBatch => ({
  type: "events",
  epoch,
  events: seqs.map((seq) => ({
    seq,
    topic: "project:web-dashboard",
    type: "card.updated",
    at: "2026-09-25T10:15:30.123Z",
    data: {},
  })),
});
