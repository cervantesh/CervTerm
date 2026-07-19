local record cervterm
  type Unset = userdata
  enum FontStyle
    "normal"
    "italic"
    "oblique"
  end

  enum FontAttributeMode
    "augment"
    "fixed"
  end

  enum FontSymbolClass
    "emoji"
    "cjk"
    "nerd_font"
    "powerline"
    "box_drawing"
    "braille"
    "symbols"
  end

  record WindowConfig
    width: integer
    height: integer
    initial_rows: integer
    initial_cols: integer
    decorations: string
    titlebar: string
    padding_x: integer
    padding_y: integer
    padding_left: integer
    padding_right: integer
    padding_top: integer
    padding_bottom: integer
    dynamic_title: boolean
    opacity: number
    text_opacity: number
    background_opacity: number
    blur: boolean
  end

  record FontDescriptor
    family: string
    collection_face: string | nil
    collection_index: integer | nil
    weight: integer | nil
    style: FontStyle | nil
    stretch: integer | nil
    attribute_mode: FontAttributeMode | nil
  end

  record FontIntegerRange
    min: integer
    max: integer
  end

  record FontRuneRange
    first: integer
    last: integer
  end

  record FontRuleMatch
    weight: FontIntegerRange | nil
    styles: {FontStyle} | nil
    stretch: FontIntegerRange | nil
    ranges: {FontRuneRange} | nil
    class: FontSymbolClass | nil
  end

  record FontRule
    match: FontRuleMatch
    use: FontDescriptor
  end

  record FontConfig
    family: string
    descriptors: {FontDescriptor} | nil
    fallback: {FontDescriptor} | nil
    rules: {FontRule} | nil
    size: number
    ligatures: boolean
    features: {string: integer} | nil
    line_height: number
    cell_width: number
    baseline_offset: number
    glyph_offset_x: number
    glyph_offset_y: number
  end

  record BackgroundLayer
    kind: string
    opacity: number | nil
    color: string | nil
    colors: {string} | nil
    angle: number | nil
    path: string | nil
    fit: string | nil
    horizontal_align: string | nil
    vertical_align: string | nil
  end

  record BackgroundDocument
    layers: {BackgroundLayer} | Unset
  end

  record ColorsConfig
    foreground: string
    background: string
    cursor: string
    selection_background: string
    chrome_background: string
    chrome_muted: string
    accent: string
    split: string
    search_match: string
    error: string
    ansi: {string}
    indexed_colors: {integer: string}
  end

  record ScrollingConfig
    history: integer
    wheel_multiplier: integer
    hide_cursor_when_scrolled: boolean
  end

  record ScrollbarConfig
    enabled: boolean
    mode: string
    stable_gutter: boolean
    animation_fps: integer
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

  record TabBarConfig
    mode: string
    position: string
    height_px: integer
    min_width_px: integer
    max_width_px: integer
    padding_x: integer
    show_new_button: boolean
    show_close_button: boolean
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
    max_fps: integer
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
    text_opacity: function(Term): number
    set_text_opacity: function(Term, number)
    background_opacity: function(Term): number
    set_background_opacity: function(Term, number)
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

  record Action
  end

  record ActionModule
    CopySelection: Action
    PasteClipboard: Action
    ToggleSearch: Action
    ToggleStats: Action
    ActivateCommandPalette: Action
    ActivateQuickSelect: Action
    ActivateLaunchMenu: Action
    ReloadConfig: Action
    ClosePane: Action
    ResetFontSize: Action
    NewTab: Action
    ActivateTabSwitcher: Action
    NewWindow: Action
    ScrollLines: function(integer): Action
    ScrollPage: function(integer): Action
    ScrollBuffer: function(integer): Action
    Zoom: function(number): Action
    SplitPane: function(string): Action
    FocusPane: function(string): Action
    ResizePane: function(string, integer): Action
    SwapPane: function(string): Action
    MovePane: function(string): Action
    ActivateTab: function(integer): Action
    ActivateTabRelative: function(integer): Action
    MoveTab: function(integer, integer): Action
    RenameTab: function(integer, string): Action
    CloseTab: function(integer): Action
    MovePaneToTab: function(integer, string): Action
    CloseWindow: function(integer): Action
    FocusWindow: function(integer): Action
    MoveTabToWindow: function(integer, integer, integer): Action
    MovePaneToWindow: function(integer, integer, string): Action
    Multiple: function({Action}): Action
    WithTarget: function(Action, string): Action
  end

  record ConfigModule
    unset: Unset
  end

  record WindowDocument
    width: integer | Unset
    height: integer | Unset
    padding_x: integer | Unset
    padding_y: integer | Unset
    padding_left: integer | Unset
    padding_right: integer | Unset
    padding_top: integer | Unset
    padding_bottom: integer | Unset
    dynamic_title: boolean | Unset
    opacity: number | Unset
    text_opacity: number | Unset
    background_opacity: number | Unset
    blur: boolean | Unset
  end

  record FontDocument
    family: string | Unset
    descriptors: {FontDescriptor} | Unset
    fallback: {FontDescriptor} | Unset
    rules: {FontRule} | Unset
    size: number | Unset
    ligatures: boolean | Unset
    features: {string: integer | Unset} | Unset
    line_height: number | Unset
    cell_width: number | Unset
    baseline_offset: number | Unset
    glyph_offset_x: number | Unset
    glyph_offset_y: number | Unset
  end

  record ColorsDocument
    foreground: string | Unset
    background: string | Unset
    cursor: string | Unset
    selection_background: string | Unset
    chrome_background: string | Unset
    chrome_muted: string | Unset
    accent: string | Unset
    split: string | Unset
    search_match: string | Unset
    error: string | Unset
    ansi: {string} | Unset
    indexed_colors: {integer: (string | Unset)} | Unset
  end

  record ColorScheme
    foreground: string | Unset
    background: string | Unset
    cursor: string | Unset
    selection_background: string | Unset
    chrome_background: string | Unset
    chrome_muted: string | Unset
    accent: string | Unset
    split: string | Unset
    search_match: string | Unset
    error: string | Unset
    ansi: {string} | Unset
    indexed_colors: {integer: (string | Unset)} | Unset
  end

  record ScrollingDocument
    history: integer | Unset
    wheel_multiplier: integer | Unset
    hide_cursor_when_scrolled: boolean | Unset
  end

  record ScrollbarDocument
    enabled: boolean | Unset
    mode: string | Unset
    stable_gutter: boolean | Unset
    animation_fps: integer | Unset
    reserved_width_px: integer | Unset
    width_px: integer | Unset
    margin_px: integer | Unset
    radius_px: integer | Unset
    min_thumb_px: integer | Unset
    track_color: string | Unset
    thumb_color: string | Unset
    thumb_hover_color: string | Unset
    thumb_press_color: string | Unset
    auto_hide_delay_ms: integer | Unset
    fade_ms: integer | Unset
    page_step: number | Unset
    track_click: string | Unset
  end

  record TabBarDocument
    mode: string | Unset
    position: string | Unset
    height_px: integer | Unset
    min_width_px: integer | Unset
    max_width_px: integer | Unset
    padding_x: integer | Unset
    show_new_button: boolean | Unset
    show_close_button: boolean | Unset
  end

  record CursorDocument
    shape: string | Unset
    blink: boolean | Unset
    blink_interval_ms: integer | Unset
    thickness: number | Unset
  end

  record ClipboardDocument
    osc52: string | Unset
  end

  record RenderDocument
    bidi: boolean | Unset
    text_gamma: number | Unset
    text_darken: number | Unset
    text_raster: string | Unset
    stats_hotkey: string | Unset
    zoom_in_hotkey: string | Unset
    zoom_out_hotkey: string | Unset
    zoom_reset_hotkey: string | Unset
    vsync: boolean | Unset
    max_fps: integer | Unset
    redraw: string | Unset
    damage: string | Unset
  end

  record ShellDocument
    program: string | Unset
    args: {string} | Unset
    working_directory: string | Unset
    env: {string:(string | Unset)} | Unset
  end

  record KeyBinding
    key: string
    mods: string | nil
    label: string | nil
    table: string | nil
    action: Action | function(Term) | nil
  end

  record LeaderBinding
    key: string
    mods: string | nil
    timeout_ms: integer | nil
  end

  record KeyTableBinding
    name: string
    one_shot: boolean | nil
    timeout_ms: integer | nil
    keys: {KeyBinding}
  end

  record MouseBinding
    event: string
    button: string
    mods: string | nil
    click_count: integer | nil
    label: string | nil
    action: Action | function(Term)
  end

  record QuickSelectRule
    id: string
    pattern: string
    action: string
    priority: integer | nil
  end

  record QuickSelectDocument
    rules: {QuickSelectRule} | Unset | nil
  end

  record LaunchTarget
    id: string
    label: string
    program: string
    args: {string} | nil
    cwd: string | nil
    env: {string:string} | nil
  end

  record Events
    output: function(Term, string) | Unset | nil
    title: function(Term, string) | Unset | nil
    cwd: function(Term, string) | Unset | nil
    bell: function(Term) | Unset | nil
    resize: function(Term, integer, integer) | Unset | nil
    focus: function(Term, boolean) | Unset | nil
    scroll: function(Term, integer) | Unset | nil
  end

  record PartialConfig
    color_scheme: string | Unset
    window: WindowDocument | Unset
    font: FontDocument | Unset
    colors: ColorsDocument | Unset
    background: BackgroundDocument | Unset | nil
    scrolling: ScrollingDocument | Unset
    scrollbar: ScrollbarDocument | Unset
    tab_bar: TabBarDocument | Unset
    cursor: CursorDocument | Unset
    clipboard: ClipboardDocument | Unset
    render: RenderDocument | Unset
    shell: ShellDocument | Unset
    launch_menu: {LaunchTarget} | Unset | nil
    quick_select: QuickSelectDocument | Unset
    keys: {KeyBinding} | Unset | nil
    leader: LeaderBinding | Unset | nil
    key_tables: {KeyTableBinding} | Unset | nil
    mouse_bindings: {MouseBinding} | Unset | nil
    events: Events | Unset | nil
  end

  record Config
    config_version: integer | nil
    includes: {string} | nil
    default_environment: string | nil
    default_profile: string | nil
    environments: {string:PartialConfig} | nil
    profiles: {string:PartialConfig} | nil
    color_scheme: string | Unset
    color_schemes: {string:ColorScheme} | nil
    window: WindowDocument | Unset
    font: FontDocument | Unset
    colors: ColorsDocument | Unset
    background: BackgroundDocument | Unset | nil
    scrolling: ScrollingDocument | Unset
    scrollbar: ScrollbarDocument | Unset
    tab_bar: TabBarDocument | Unset
    cursor: CursorDocument | Unset
    clipboard: ClipboardDocument | Unset
    render: RenderDocument | Unset
    shell: ShellDocument | Unset
    launch_menu: {LaunchTarget} | Unset | nil
    quick_select: QuickSelectDocument | Unset
    keys: {KeyBinding} | Unset | nil
    leader: LeaderBinding | Unset | nil
    key_tables: {KeyTableBinding} | Unset | nil
    mouse_bindings: {MouseBinding} | Unset | nil
    events: Events | Unset | nil
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

  config: ConfigModule
  action: ActionModule
  after: function(integer, function(Term)): integer
  every: function(integer, function(Term)): integer
  cancel: function(integer)
  status: function(string, string)
  overlay: function(string): Overlay
end
return cervterm
