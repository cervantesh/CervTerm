//go:build glfw && darwin

#import <Cocoa/Cocoa.h>
#include <stdlib.h>

struct cervterm_visual_effect {
    NSWindow *window;
    NSView *original_view;
    NSView *wrapper_view;
    NSVisualEffectView *effect_view;
    NSAutoresizingMaskOptions original_autoresizing_mask;
};

struct cervterm_visual_effect *cervterm_visual_effect_create(void *ns_window, int *status_out) {
    @autoreleasepool {
        if (status_out == NULL) {
            return NULL;
        }
        *status_out = 0;
        NSWindow *window = (NSWindow *)ns_window;
        NSView *original = [window contentView];
        if (window == nil || original == nil) {
            *status_out = -1;
            return NULL;
        }

        struct cervterm_visual_effect *context = calloc(1, sizeof(*context));
        if (context == NULL) {
            *status_out = -2;
            return NULL;
        }

        NSView *wrapper = [[NSView alloc] initWithFrame:[original frame]];
        NSVisualEffectView *effect = [[NSVisualEffectView alloc] initWithFrame:[wrapper bounds]];
        if (wrapper == nil || effect == nil) {
            [effect release];
            [wrapper release];
            free(context);
            *status_out = -3;
            return NULL;
        }

        [wrapper setAutoresizingMask:NSViewWidthSizable | NSViewHeightSizable];
        [effect setAutoresizingMask:NSViewWidthSizable | NSViewHeightSizable];
        [effect setMaterial:NSVisualEffectMaterialUnderWindowBackground];
        [effect setBlendingMode:NSVisualEffectBlendingModeBehindWindow];
        [effect setState:NSVisualEffectStateFollowsWindowActiveState];
        [effect setHidden:YES];

        context->window = window;
        context->original_view = [original retain];
        context->wrapper_view = wrapper;
        context->effect_view = effect;
        context->original_autoresizing_mask = [original autoresizingMask];

        [window setContentView:wrapper];
        [original setFrame:[wrapper bounds]];
        [original setAutoresizingMask:NSViewWidthSizable | NSViewHeightSizable];
        [wrapper addSubview:effect];
        [wrapper addSubview:original positioned:NSWindowAbove relativeTo:effect];
        [window makeFirstResponder:original];
        return context;
    }
}

int cervterm_visual_effect_set(struct cervterm_visual_effect *context, int enabled) {
    @autoreleasepool {
        if (context == NULL || context->effect_view == nil) {
            return -1;
        }
        [context->effect_view setHidden:enabled == 0];
        [context->effect_view setNeedsDisplay:YES];
        return 0;
    }
}

void cervterm_visual_effect_destroy(struct cervterm_visual_effect *context) {
    @autoreleasepool {
        if (context == NULL) {
            return;
        }
        [context->effect_view setHidden:YES];
        if ([context->window contentView] == context->wrapper_view) {
            [context->original_view removeFromSuperviewWithoutNeedingDisplay];
            [context->window setContentView:context->original_view];
            [context->original_view setFrame:[context->wrapper_view bounds]];
            [context->original_view setAutoresizingMask:context->original_autoresizing_mask];
            [context->window makeFirstResponder:context->original_view];
        }
        [context->effect_view removeFromSuperviewWithoutNeedingDisplay];
        [context->effect_view release];
        [context->wrapper_view release];
        [context->original_view release];
        free(context);
    }
}
