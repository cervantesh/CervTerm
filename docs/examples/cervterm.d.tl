local record cervterm
  record Term
    write: function(Term, string)
    notify: function(Term, string)
  end
  record Events
    output: function(Term, string)
    title: function(Term, string)
    bell: function(Term)
  end
end
return cervterm
