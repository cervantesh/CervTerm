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
  after: function(integer, function(Term)): integer
  every: function(integer, function(Term)): integer
  cancel: function(integer)
end
return cervterm
