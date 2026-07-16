local record cervterm
  record WindowConfig
    width: integer
    height: integer
    padding_x: integer
    padding_y: integer
    dynamic_title: boolean
    opacity: number
    blur: boolean
  end

  record FontConfig
    family: string
    size: number
    ligatures: boolean
  end

  record ColorsConfig
    foreground: string
    background: string
    cursor: string
    selection_background: string
  end

  record ScrollingConfig
    history: integer
    wheel_multiplier: integer
    hide_cursor_when_scrolled: boolean
  end

  record ScrollbarConfig
    enabled: boolean
    reserved_width_px: integer
    width_px: integer
    margin_px: integer
    radius_px: integer
    min_thumb_px: integer
    track_color: string
    thumb_color: string
    thumb_hover_color: string
    thumb_press_color: string
    auto_hide_delay_ms: integer
    fade_ms: integer
    page_step: number
    track_click: string
  end

  record CursorConfig
    shape: string
    blink: boolean
    blink_interval_ms: integer
    thickness: number
  end

  record ClipboardConfig
    osc52: string
  end

  record RenderConfig
    bidi: boolean
    text_gamma: number
    text_darken: number
    text_raster: string
    stats_hotkey: string
    zoom_in_hotkey: string
    zoom_out_hotkey: string
    zoom_reset_hotkey: string
    vsync: boolean
    redraw: string
    damage: string
  end

  record ShellConfig
    program: string
    args: {string}
    working_directory: string
    env: {string:string}
  end

  record Term
    write: function(Term, string)
    notify: function(Term, string)
    selection: function(Term): string
    copy: function(Term, string)
    clipboard: function(Term): string
    scroll: function(Term, integer): boolean
    scroll_to_bottom: function(Term)
    scrollback: function(Term): integer
    size: function(Term): integer, integer
    cursor: function(Term): integer, integer
    title: function(Term): string
    cwd: function(Term): string
    set_title: function(Term, string)
    line: function(Term, integer): string
    line_wrapped: function(Term, integer): boolean
    font_size: function(Term): number
    set_font_size: function(Term, number)
    search: function(Term, string): boolean
    window_opacity: function(Term): number
    set_window_opacity: function(Term, number)
    background: function(Term): string
    set_background: function(Term, string)
    blur: function(Term): boolean
    set_blur: function(Term, boolean)
    scrolling: function(Term): ScrollingConfig
    set_scrolling: function(Term, ScrollingConfig)
    scrollbar: function(Term): ScrollbarConfig
    set_scrollbar: function(Term, ScrollbarConfig)
    reload_config: function(Term): boolean
  end

  record KeyBinding
    key: string
    mods: string
    action: function(Term)
  end

  record Events
    output: function(Term, string) | nil
    title: function(Term, string) | nil
    cwd: function(Term, string) | nil
    bell: function(Term) | nil
    resize: function(Term, integer, integer) | nil
    focus: function(Term, boolean) | nil
    scroll: function(Term, integer) | nil
  end

  record Config
    window: WindowConfig
    font: FontConfig
    colors: ColorsConfig
    scrolling: ScrollingConfig
    scrollbar: ScrollbarConfig
    cursor: CursorConfig
    clipboard: ClipboardConfig
    render: RenderConfig
    shell: ShellConfig
    keys: {KeyBinding} | nil
    events: Events | nil
  end

  -- A retained, cell-addressed display list drawn on top of the terminal.
  -- Coordinates are 1-based cells; colors are "#RRGGBB" or "#RRGGBBAA".
  -- Primitives are built into a pending list and shown atomically by commit.
  record Overlay
    clear: function(Overlay)
    rect: function(Overlay, integer, integer, integer, integer, string)
    text: function(Overlay, integer, integer, string, string)
    hline: function(Overlay, integer, integer, integer, string)
    vline: function(Overlay, integer, integer, integer, string)
    commit: function(Overlay)
    show: function(Overlay)
    hide: function(Overlay)
    destroy: function(Overlay)
  end

  after: function(integer, function(Term)): integer
  every: function(integer, function(Term)): integer
  cancel: function(integer)
  status: function(string, string)
  overlay: function(string): Overlay
end
return cervterm
