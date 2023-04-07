# Why not?

## What?

A Markdown renderer in Go using `ebitenengine` for rendering and `goldmark` for parsing.  Use the mouse wheel to scroll up and down the document.  Resize the window to let it reflow.

## Why?

Why not?

## Features

Here is an overview of the Markdown features that are implemented.
1. Headings
2. Paragraphs
3. Highlighting
4. Inline code
5. Code blocks
6. Ordered and unorderd lists

Here are some features that are not yet implemented
* Nested lists
* Links
* Images

## Examples

Six levels of headers are supported

### Level 3 Heading

Lorem ipsum dolor sit amet, *consectetur adipiscing* elit, sed do __eiusmod tempor__ incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id ___est laborum___.

#### Level 4 Heading

Lorem ipsum dolor sit amet, *consectetur adipiscing* elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.

##### Level 5 Heading

###### Level 6 Heading with *highlight*

You can have `inline code` and you can have inline code in highlighted text, e.g. **this is an important `identifier.`** Code blocks are also supported. For example, this is how you can implement _Fibonacci_ in Python.

```
def fib(n, a = 0, b = 1):
    while n > 0:
        a, b = b, a + b
    return a
```

This is how a list with long items looks like.

1. First item. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.
2. Second item. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.
3. Third item
4. Another item
5. Another item
6. Another item
7. Another item
8. Another item
9. Another item
10. 10th item

## Cute!

![cat.jpg](cat.jpeg "lovely cat")