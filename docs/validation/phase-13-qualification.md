# Phase 13 Terminal-Image Qualification

## Disposition

Phase 13 is **experimental, default-off, restart-scoped, and subset-only**. Automated evidence passes for the bounded model, direct-data Kitty adapter, owner-thread runtime, detached projection, OpenGL fake capability/cache, drawing, rollback, and disabled fast path. The real Windows/OpenGL visual matrix remains **UNRUN**, so this report makes no stable platform or full-Kitty conformance claim. Operational rollback is `graphics.kitty.enabled=false` followed by restart.

## Qualification matrix

| Requirement | Result | Reproducible evidence |
|---|---|---|
| RGB24 and RGBA32 | Pass | `internal/kitty/decode_test.go:TestDecodeRawRGBAAndRGBSealed` |
| Raw zlib | Pass | `internal/kitty/decode_test.go:TestDecodeZlibRGBA`, `TestDecodeZlibBombAndTrailingRollback` |
| PNG | Pass | `internal/kitty/decode_test.go:TestDecodePNGAndRollbackInvalid` |
| One-shot and chunked direct data | Pass | `internal/kitty/adapter_test.go:TestAdapterOneShotTransferOwnership`, `TestAdapterChunkingPreservesOrderAndRollsBackMalformed`, `TestAdapterFragmentationAndCancellation` |
| Transmit / replace | Pass | `internal/core/images_test.go:TestPrivateImageCommitTransmitPlaceReplaceAndDelete`, `internal/termimage/prepared_test.go:TestPreparedReplacementAndRemovalReleaseAfterFinalize` |
| Transmit-and-place / place / crop / opacity / z | Pass | `internal/core/images_api_external_test.go:TestPublicImageAPICommitProjectionDeleteAndReset`, `internal/frontend/glfwgl/terminal_image_draw_test.go:TestTerminalImageFrameResolvesCropFullSourceOpacityAndDestinationGeometry`, `TestTerminalImagePaneDispatchGoldenPreservesClipAndSemanticLayerOrder` |
| Delete selectors | Pass | `internal/termimage/placement_test.go:TestDeleteSelectorTruthTableAndCopiesIDs`, `internal/core/images_test.go:TestImageDeleteSelectorScopesAndResourceExpansion` |
| Query and ordered replies | Pass | `internal/mux/mux_kitty_test.go:TestKittyRuntimeAsyncReplyPrecedesLaterDSRAndCommits`, `TestKittyRuntimeSynchronousReplyPreservesParserOrder`; bounded/redacted policy: `internal/kitty/reply_test.go` |
| Malformed, short, long, trailing and unknown input | Pass | `internal/kitty/header_test.go:TestParseRejectsDuplicatesUnknownExternalAndConflicts`, `internal/kitty/decode_test.go:TestDecodeRejectsMalformedShortLongAndTrailing` |
| Overflow and reservation rollback | Pass | `internal/kitty/adapter_test.go:TestAdapterOversizeDiscardsUntilAPCTerminator`, `internal/termimage/budget_test.go:TestCompositeReservationRollsBackEarlierCounters`, `internal/core/images_test.go:TestImageCommitIsOldOrNewAndFaultsRollback` |
| Cancellation and EOF | Pass | `internal/kitty/decode_test.go:TestDecodeJobCancellationAndClose`, `internal/mux/mux_kitty_test.go:TestKittyRuntimeEOFDiscardsPartialTransfer` |
| Transfer/decode deadlines | Pass | `internal/kitty/adapter_test.go:TestAdapterExpiryIsPureBoundedAndIdempotent`, `internal/mux/mux_kitty_test.go:TestKittyRuntimeAcceptanceDeadlineUnblocksOrderedReplies`, `TestKittyRuntimeDeadlineExpiresWithoutIngress` |
| Pane/process saturation and worker ownership | Pass | `internal/kitty/adapter_test.go:TestAdapterPaneAndProcessPendingTransferBounds`, `internal/mux/kitty_decode_scheduler_test.go:TestKittyDecodeSchedulerBoundsProcessConcurrency`, `TestKittyDecodeSchedulerRejectsSaturatedQueue`, `TestKittyDecodeSchedulerCloseSubmitRaceCleansEveryJobExactlyOnce` |
| Erase/edit lifecycle | Pass | `internal/core/images_lifecycle_test.go:TestImageLifecycleOverwriteAndEraseDeleteWholePlacement`, `TestImageLifecycleInsertDeleteCharacters`, boundary matrices in the same file |
| Scroll, history, ED3 and eviction | Pass | `internal/core/images_lifecycle_test.go:TestImageLifecycleFullScrollHistoryAndRingEviction`, `TestImageLifecycleCapacityReductionAndED3Rebase`, `TestImageViewportPinnedOutputAndEviction` |
| Repeated reflow | Pass | `internal/core/images_reflow_test.go:TestImagePrimaryRepeatedNarrowWideReflowDeterministic`, `TestImageHistoryLiveStraddleReflowPreservesBothSides` |
| Alternate screen and reset/RIS | Pass | `internal/core/images_reflow_test.go:TestImageAlternateResizeCropsAnchorOnlyAndExitDiscards`, `internal/vt/image_ris_test.go:TestRISResetsImageEpochAndParserRemainsReusable`, `internal/mux/mux_kitty_test.go:TestKittyRuntimeResetRejectsStaleCompletionAndReleasesCandidate` |
| Pane/tab/window transfer | Pass | `internal/mux/mux_images_test.go:TestMuxCrossWindowTransferPreservesImageStoreAndResource`, `TestMuxCrossWindowTabTransferPreservesEveryImageStoreAndResource` |
| Restore and projection rollback | Pass | `internal/frontend/glfwgl/terminal_image_activation_test.go:TestTerminalImageActivationCreatesInitialChildAndRestoreCaches`, `TestTerminalImageActivationRestorePreparationAndBindFailuresCloseEveryCache` |
| Close and shared-budget release | Pass | `internal/mux/mux_images_test.go:TestMuxShutdownReleasesSharedImageBudget`, `internal/termimage/prepared_adversarial_test.go:TestResetAndCloseAbortPreparedWithoutResurrection` |
| Exact generation and detached snapshot | Pass | `internal/mux/mux_images_test.go:TestMuxAcquireImageResourceChecksPaneAndGenerationAndDetaches`, `internal/mux/image_snapshot_test.go:TestPaneViewDeepDetachesImageAndSnapshotMetadata`, `internal/render/image_snapshot_test.go` |
| GL upload validation/failure | Pass (fake GL) | `internal/frontend/glfwgl/terminal_image_test.go:TestPrepareTerminalImageRejectsMalformedResourceBeforeGLCalls`, `TestPrepareTerminalImageReportsTextureCreationFailure`, `TestTerminalImageTextureCloseIsIdempotentAndInvalidatesBinding` |
| Context-local cache, caps, retry and teardown | Pass (fake GL) | `internal/frontend/glfwgl/terminal_image_cache_test.go:TestTerminalImageCacheIsProjectionLocalAndModelIndependent`, `TestTerminalImageCacheLimitsAreHardAndLowerOnly`, `TestTerminalImageCacheRetriesSameGenerationOnFixedScheduleAndThenStops`, `TestTerminalImageCacheClosesWithCurrentContextBeforeRendererAcrossProjectionLifecycles` |
| Context loss on a real driver | UNRUN / unclaimed | Manual Windows/OpenGL row in `docs/manual-verification.md`; support remains experimental/default-off and manual qualification is not claimed. |
| Pane clip, z order, multi-pane omission | Pass (recording/fake renderer) | `internal/frontend/glfwgl/terminal_image_draw_test.go:TestTerminalImagePaneDispatchGoldenPreservesClipAndSemanticLayerOrder`, `TestTerminalImageFrameDrawsOnlyRequestedPane`, `TestTerminalImageFrameDeterministicallyOmitsMissingTexture` |
| Image-only pane damage | Pass | `internal/render/image_snapshot_test.go:TestImageGenerationIsIndependentFromTextRowHashes`, `internal/frontend/glfwgl/terminal_image_draw_test.go:TestTerminalImageDamageTracksGenerationPerPaneForBothBackBuffersWithoutRowHashChanges`, `TestTerminalImageDamageOnlyAcceptsExactUploadedKeyReferences` |
| Disabled allocation and idle cadence | Pass | `internal/core/images_lifecycle_test.go:TestNilImageLifecycleLeavesTextPathAllocationFree`, `internal/render/image_snapshot_test.go:TestTextOnlyCaptureKeepsNilImagesAllocationFree`, `internal/frontend/glfwgl/phase13_frame_benchmark_test.go:TestPhase13DisabledFrameIsAllocationAndMutationFree`, `TestPhase13DisabledFrameAddsNoRedrawOrIdleCadence` |
| Default-off activation and transactional rollback | Pass | `internal/frontend/glfwgl/terminal_image_activation_test.go:TestTerminalImageActivationDisabledIsLiteralNil`, `TestTerminalImageActivationCommitFailureClosesCacheThenMux`, `TestTerminalImageActivationRuntimeBindFailureClosesCacheAndMuxWindow` |
| Real Windows/OpenGL visual behavior | UNRUN / unclaimed | Exact matrix and evidence fields are in `docs/manual-verification.md#phase-13-experimental-kitty-images--windowsopengl-qualification`. |

## Explicit exclusions

The following are rejected or unimplemented, not `N/A`: filesystem/path/temporary-file/shared-memory transports, remote reads/writes, animation/frame composition, Unicode placeholders, Sixel DCS, iTerm OSC 1337, renderer selection, and broad Kitty application conformance. See ADR-0014 and `docs/spec.md#phase-13-experimental-kitty-terminal-graphics`.
