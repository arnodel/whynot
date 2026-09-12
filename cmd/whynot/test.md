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
5. Fenced and indented code blocks
6. Ordered and unorderd lists
7. Images
8. Thematic breaks
9. Links and autolinks
10. Blockquotes
11. Strikethrough
12. Nested lists

Here are some features that are not yet implemented
* Tables

> This is a blockquote. It can contain *emphasis*, `inline code`, and
> wraps just like a normal paragraph.
>
> > Blockquotes can nest too.

See it in action: [the project's GitHub repo](https://github.com/arnodel/whynot),
or just visit <https://example.com>.

Local links work too, and can be followed the same way: here's
[a much larger test document](../../testdata/test-large.md), and here's
[the project's README](../../README.md).

## Examples

Six levels of headers are supported

### Level 3 Heading

Lorem ipsum dolor sit amet, *consectetur adipiscing* elit, sed do __eiusmod tempor__ incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id ___est laborum___.

#### Level 4 Heading

Lorem ipsum dolor sit amet, *consectetur adipiscing* elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.

##### Level 5 Heading

###### Level 6 Heading with *highlight*

You can have `inline code` and you can have inline code in highlighted text, e.g. **this is an important `identifier.`** You can also have ~~struck-through text~~, even combined with **~~bold strikethrough~~**. Code blocks are also supported. For example, this is how you can implement _Fibonacci_ in Python.

```
def fib(n, a = 0, b = 1):
    while n > 0:
        a, b = b, a + b
    return a
```

Code blocks can also be indented instead of fenced.

    def fib(n, a = 0, b = 1):
        while n > 0:
            a, b = b, a + b
        return a

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

Task lists work too.

- [x] Write the renderer
- [x] Support task lists
- [ ] Support tables

Lists can nest, to any depth.

- Fruit
  - Apple
  - Banana
    - Cavendish
    - Plantain
- Vegetables
  - Carrot

Nesting also works with numbered lists, mixed marker styles at each
level, and with items whose own text is long enough to wrap before the
nested list underneath them begins.

1. First step. This item has a fairly long piece of explanatory text
   attached to it, long enough that it should wrap onto a second line
   before the nested list underneath it begins, so we can check that
   the wrap and the nested list both line up under the same indentation.
   - Sub-point one
   - Sub-point two
2. Second step
   1. Numbered sub-step one
   2. Numbered sub-step two
      * Deeply nested bullet, three levels down
      * Another one, to check spacing between deeply nested siblings
3. Third step
   * Bulleted sub-step, under a numbered parent
   * Another bulleted sub-step

A loose list - items separated by a blank line - gets real paragraph
spacing between items, instead of a tight list's compact spacing.

- First item, loose because of the blank lines around it.

- Second item, same reason.

- Third item, with a second paragraph of its own, indented to stay
  part of the same item.

  This second paragraph should also get real paragraph spacing, both
  above (from the first paragraph) and below (from the next item).

- Fourth and last item.

A thematic break separates sections, like the one below.

---

## Tables

A basic table, with a header row:

| Fruit  | Color  | Price |
|--------|--------|-------|
| Apple  | Red    | $1    |
| Banana | Yellow | $0.50 |
| Kiwi   | Green  | $1.50 |

Column alignment - left, center, and right:

| Left | Center | Right |
|:-----|:------:|------:|
| a    | b      | c     |
| dd   | ee     | ff    |

A column whose content is much longer than the others, plus some inline
formatting inside cells:

| Item  | Description                                                                                             | Qty |
|-------|----------------------------------------------------------------------------------------------------------|----:|
| `foo` | A **very** long description, long enough to show how equal-width columns cope with one much wider column |   3 |
| bar   | short                                                                                                     |  12 |

Two columns both needing to wrap, sharing the squeeze between them
rather than one column dominating:

| Name     | Pros                                                                | Cons                                                       |
|----------|---------------------------------------------------------------------|-------------------------------------------------------------|
| Option A | Fast, cheap, and easy to set up with minimal configuration required | Limited flexibility for advanced use cases down the line   |
| Option B | Highly configurable and extensible for complex scenarios            | Steeper learning curve and more time to get started         |

Several narrow columns next to one wide one:

| ID | Status | Owner | Notes                                                                                                          |
|----|--------|-------|------------------------------------------------------------------------------------------------------------------|
| 1  | OK     | Al    | All good                                                                                                        |
| 2  | FAIL   | Bo    | This one needs a much longer explanation of what went wrong and why, spanning well past the width of the other columns |
| 3  | OK     | Cy    | Fine                                                                                                            |

Four columns, all roughly equally demanding - no column is narrow
enough to stay unconstrained, so every one shrinks together:

| First column with a fair bit of text  | Second column also fairly wordy   | Third one too, about the same length | Fourth, similar again          |
|----------------------------------------|-------------------------------------|-----------------------------------------|-----------------------------------|
| Some more moderate length content here | And here as well, similar length   | Roughly matching length once more       | And the last one too              |

A table with a header but no data rows:

| A | B |
|---|---|

## Cute!

![cat.jpg](cat.jpeg "lovely cat")