import "@testing-library/jest-dom/vitest";
import { setZone } from "~/data/zone";
import { installTestStore } from "~/testing/install-test-store";

installTestStore();

// A test that loads the daemon's profile sets the app's zone; the next test starts on the device's.
afterEach(() => setZone(""));
