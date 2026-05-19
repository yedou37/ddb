#set page(
  paper: "a4",
  margin: (
    top: 2.54cm,
    bottom: 2.54cm,
    left: 2.54cm,
    right: 2.54cm,
  ),
)

#set text(
  lang: "zh",
  font: ("Inter", "Google Sans", "Noto Sans CJK SC", "Source Han Sans SC", "PingFang SC"),
  size: 10.5pt,
  fill: rgb("#202124"),
)

#set par(
  justify: true,
  first-line-indent: 2em,
  leading: 0.82em,
)

#let google-blue = rgb("#1A73E8")
#let google-text = rgb("#202124")
#let google-subtle = rgb("#5F6368")
#let google-border = rgb("#DADCE0")
#let google-header = rgb("#F1F3F4")
#let google-gray = rgb("#F8F9FA")

#let sans = ("Inter", "Google Sans", "Noto Sans CJK SC", "Source Han Sans SC", "PingFang SC")

#let fill-line(width: 7cm) = box(
  width: width,
  height: 1.15em,
  inset: 0pt,
  stroke: (bottom: 0.8pt + google-border),
)[]

#let toc-line(title, page) = grid(
  columns: (78%, 22%),
  column-gutter: 0pt,
  [
    #text(fill: google-text)[#title]
  ],
  [
    #align(right)[#text(fill: google-subtle)[#page]]
  ],
)

#let h1(title) = block(above: 1.1em, below: 0.6em)[
  #box(width: 100%, inset: (left: 10pt, top: 7pt, bottom: 7pt), fill: google-header, radius: 6pt)[
    #text(font: sans, size: 15.5pt, weight: "bold", fill: google-blue)[#title]
  ]
]

#let h2(title) = block(above: 0.9em, below: 0.5em)[
  #text(font: sans, size: 13pt, weight: "bold", fill: google-text)[#title]
]

#let h3(title) = block(above: 0.8em, below: 0.4em)[
  #text(font: sans, size: 11.5pt, weight: "semibold", fill: google-subtle)[#title]
]

#let body-line(text) = block(below: 0.18em)[#text]

#let divider(width: 100%) = box(
  width: width,
  height: 0pt,
  stroke: (bottom: 0.8pt + google-border),
)[]

#align(center)[
  #v(18mm)
  #box(
    width: 100%,
    inset: (left: 16pt, right: 16pt, top: 16pt, bottom: 16pt),
    fill: google-gray,
    stroke: 0.8pt + google-border,
    radius: 12pt,
  )[
    #align(left)[
      #text(font: sans, size: 9pt, weight: "medium", fill: google-blue)[COURSE REPORT TEMPLATE]
      #v(8pt)
      #text(font: sans, size: 24pt, weight: "bold", fill: google-text)[大规模信息系统构建技术导论]
      #v(10pt)
      #text(font: sans, size: 18pt, weight: "bold", fill: google-blue)[分布式MiniSQL系统模块实现与测试报告]
      #v(12pt)
      #divider()
      #v(18pt)
      #grid(
        columns: (2.1cm, 1fr),
        row-gutter: 10pt,
        column-gutter: 10pt,
        align: (left, left),
        [#text(font: sans, size: 11pt, weight: "medium", fill: google-subtle)[姓名]], [#fill-line(width: 100%)],
        [#text(font: sans, size: 11pt, weight: "medium", fill: google-subtle)[学院]], [#fill-line(width: 100%)],
        [#text(font: sans, size: 11pt, weight: "medium", fill: google-subtle)[系]],   [#fill-line(width: 100%)],
        [#text(font: sans, size: 11pt, weight: "medium", fill: google-subtle)[专业]], [#fill-line(width: 100%)],
        [#text(font: sans, size: 11pt, weight: "medium", fill: google-subtle)[学号]], [#fill-line(width: 100%)],
      )
      #v(22pt)
      #align(right)[#text(font: sans, size: 11pt, fill: google-subtle)[2026 年 5 月  日]]
    ]
  ]
]

#pagebreak()
#counter(page).update(2)
#set page(
  footer: context align(center)[
    #box(
      width: 45%,
      height: 0pt,
      stroke: (bottom: 0.7pt + google-border),
    )[]
    #v(4pt)
    #text(font: sans, size: 9pt, fill: google-subtle)[#counter(page).display()]
  ],
)

#align(center)[#text(font: sans, size: 16pt, weight: "bold", fill: google-blue)[目录]]

#v(0.9em)
#divider()
#v(0.8em)
#toc-line([一．系统模块简介], [3])
#toc-line([二．xxx模块实现说明], [3])
#toc-line([2.1 模块组件设计], [3])
#toc-line([2.2 主要数据结构], [4])
#toc-line([2.3 流程图设计], [4])
#toc-line([三．yyy模块实现说明], [4])
#toc-line([3.1 模块组件设计], [4])
#toc-line([3.2 主要数据结构], [4])
#toc-line([3.3 流程图设计], [4])
#toc-line([四．测试结果], [5])
#toc-line([4.1 xxx功能测试], [5])
#toc-line([4.1.1 测试用例], [5])
#toc-line([4.1.2 测试结果], [5])
#toc-line([4.2 yyy功能测试], [5])
#toc-line([4.2.1 测试用例], [5])
#toc-line([4.2.2 测试结果], [5])
#toc-line([五．开发体会], [5])
#toc-line([参考文献], [5])

#pagebreak()

#h1([一．系统模块简介])
#body-line([本人在分布式MiniSQL系统中负责研发xxx、yyy和zzz模块，采用xx程序设计语言，在xx平台下编辑、编译与调试……。])
#body-line([xxx模块的具体功能如下：])
#body-line([（1）])
#body-line([（2）])
#body-line([（3）])
#body-line([（4）])
#body-line([（5）])
#body-line([……])
#body-line([（正文字号小四，行距1.5倍）])

#h1([二．xxx模块实现说明])
#h2([2.1 模块组件设计])
#body-line([本模块包括以下几个部分：])
#body-line([（1）])
#body-line([（2）])
#body-line([（3）])
#body-line([（4）])
#body-line([（5）])
#body-line([……])

#h2([2.2 主要数据结构])
#body-line([（类图、对象图等UML图）])

#h2([2.3 流程图设计])
#body-line([(序列图、状态机、活动图等UML图) :])
#body-line([……])

#h1([三．yyy模块实现说明])
#h2([3.1 模块组件设计])
#h2([3.2 主要数据结构])
#h2([3.3 流程图设计])
#body-line([……])

#h1([四．测试结果])
#h2([4.1 xxx功能测试])
#h3([4.1.1 测试用例])
#h3([4.1.2 测试结果])

#h2([4.2 yyy功能测试])
#h3([4.2.1 测试用例])
#h3([4.2.2 测试结果])

#h1([五．开发体会])

#h1([参考文献])
#body-line([[1] ])
#body-line([[2] ])
#body-line([[3] ])
