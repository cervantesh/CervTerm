local record cervterm
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
  end
  record Events
    output: function(Term, string)
    title: function(Term, string)
    cwd: function(Term, string)
    bell: function(Term)
    resize: function(Term, integer, integer)
    focus: function(Term, boolean)
    scroll: function(Term, integer)
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
  -- Set or replace an ordered status segment; empty text removes it.
  status: function(string, string)
  -- Create or fetch a retained overlay by id.
  overlay: function(string): Overlay
end
return cervterm
