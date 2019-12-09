package vuessr

import (
	"golang.org/x/net/html"
	"os"
	"regexp"
	"strings"
)

type VueElement struct {
	IsRoot           bool // 是否是根节点, 指的是<template>下一级节点, 这个节点会继承父级传递下来的class/style
	TagName          string
	Text             string
	Attrs            map[string]string // 除去指令/props/style/class之外的属性
	AttrsKeys        []string          // 属性的key, 用来保证顺序
	Directives       []Directive       // 自定义指令, 运行时
	ElseIfConditions []ElseIf          // 将与if指令匹配的elseif/else关联在一起
	Class            []string          // 静态class
	Style            map[string]string // 静态style
	StyleKeys        []string          // 样式的key, 用来保证顺序
	Props            Props             // props, 包括动态的class和style
	Children         []*VueElement     // 子节点
	VIf              *VIf              // 处理v-if需要的数据
	VFor             *VFor
	VSlot            *VSlot
	VElse            bool // 如果是VElse节点则不会生成代码(而是在vif里生成代码)
	VElseIf          bool
	VHtml            string
	VText            string
}

type Directive struct {
	Name  string // v-animate
	Value string // {'a': 1}
	Arg   string // v-set:arg
}

type ElseIf struct {
	Types      string // else / elseif
	Condition  string // elseif语句的condition表达式
	VueElement *VueElement
}

type VIf struct {
	Condition string // 条件表达式
	ElseIf    []*ElseIf
}

func (p *VIf) AddElseIf(v *ElseIf) {
	p.ElseIf = append(p.ElseIf, v)
}

type VFor struct {
	ArrayKey string
	ItemKey  string
	IndexKey string
}

type VSlot struct {
	SlotName string
	PropsKey string
}

type Props map[string]string

func (p Props) Get(key string) string {
	if p == nil {
		return ""
	}
	return p[key]
}

func (p Props) Omit(key ...string) Props {
	kMap := map[string]struct{}{}
	for _, k := range key {
		kMap[k] = struct{}{}
	}

	a := Props{}
	for k, v := range p {
		if _, ok := kMap[k]; ok {
			continue
		}
		a[k] = v
	}
	return a
}

func (p Props) Only(key ...string) Props {
	kMap := map[string]struct{}{}
	for _, k := range key {
		kMap[k] = struct{}{}
	}

	a := Props{}
	for k, v := range p {
		if _, ok := kMap[k]; !ok {
			continue
		}

		a[k] = v
	}
	return a
}

func (p Props) CanBeAttr() Props {
	html := map[string]struct{}{
		"id":  {},
		"src": {},
	}

	a := Props{}
	for k, v := range p {
		if _, ok := html[k]; !ok {
			continue
		}

		if !strings.HasPrefix(k, "data-") {
			continue
		}

		a[k] = v
	}
	return a
}

type Element struct {
	Text     string // 只是字
	TagName  string
	Attrs    []html.Attribute
	Children []*Element
	// 是否是root节点
	// 正常情况下template下的第一个节点是root节点, 如 template > div.
	// 如果没有按照vue组件的写法来写组件(template下只能有一个元素), 则所有元素都不会被当为root节点
	Root bool
}

func hNodeToElement(nodes []*html.Node) []*Element {
	var es []*Element
	for _, node := range nodes {
		text := ""
		tagName := ""

		omitNode := false
		switch node.Type {
		case html.TextNode:
			// html中多个空格和\n都应该被替换为空格
			text = strings.ReplaceAll(node.Data, "\n", " ")
			reg := regexp.MustCompile(`\s+`)
			text = reg.ReplaceAllString(text, " ")

			// 忽略空节点
			if text == " " || text == "" {
				omitNode = true
				break
			}

			tagName = "__string"
		case html.DocumentNode:
			tagName = node.Data
		case html.ElementNode:
			tagName = node.Data
		default:
			panic(node)
		}

		if omitNode {
			continue
		}

		var cs []*Element
		if node.FirstChild != nil {
			c := node.FirstChild
			var allC []*html.Node
			for c != nil {
				allC = append(allC, c)
				c = c.NextSibling
			}

			cs = hNodeToElement(allC)
		}

		es = append(es, &Element{
			Text:     text,
			TagName:  tagName,
			Attrs:    node.Attr,
			Children: cs,
		})
	}
	return es
}

// parse HTML
func parseHtml(filename string) ([]*Element, error) {
	file, err := os.Open(filename)

	if err != nil {
		panic(err)
	}

	root := &html.Node{
		Type: html.ElementNode,
	}
	node, err := html.ParseFragment(file, root)
	if err != nil {
		return nil, err
	}

	e := hNodeToElement(node)
	return e, nil
}

func ParseVue(filename string) (v *VueElement, err error) {
	es, err := parseHtml(filename)
	if err != nil {
		return
	}

	p := VueElementParser{}
	if len(es) == 1 {
		// 按照vue组件写法才会有root节点
		if es[0].TagName == "template" {
			if len(es[0].Children) == 1 {
				es[0].Children[0].Root = true
			}
		}
		v = p.Parse(es[0])
	} else {
		// 如果是多个节点, 则自动添加template包裹
		// 这种情况下不会存在root节点
		e := &Element{
			TagName:  "template",
			Children: es,
		}
		v = p.Parse(e)
	}
	return
}

type VueElementParser struct {
}

func (p VueElementParser) Parse(e *Element) *VueElement {
	vs := p.parseList([]*Element{e})
	return vs[0]
}

// 递归处理同级节点
func (p VueElementParser) parseList(es []*Element) []*VueElement {
	vs := make([]*VueElement, len(es))

	var ifVueEle *VueElement
	for i, e := range es {
		props := map[string]string{}
		var ds []Directive
		var class []string
		style := map[string]string{}
		var styleKeys []string
		attrs := map[string]string{}
		var attrsKeys []string
		var vIf *VIf
		var vFor *VFor
		var vSlot *VSlot

		// 标记节点是不是if
		var vElse *ElseIf
		var vElseIf *ElseIf

		var vHtml string
		var vText string

		for _, attr := range e.Attrs {
			oriKey := attr.Key
			ss := strings.Split(oriKey, ":")
			nameSpace := "-"
			key := oriKey
			if len(ss) == 2 {
				key = ss[1]
				nameSpace = ss[0]
			}

			if nameSpace == "v-bind" || nameSpace == "" {
				// v-bind & shorthands
				props[key] = attr.Val
			} else if strings.HasPrefix(oriKey, "v-") {
				// 指令
				// v-if=""
				// v-slot:name=""
				// v-else-if=""
				// v-else
				// v-html
				switch {
				case key == "v-for":
					val := attr.Val

					ss := strings.Split(val, " in ")
					arrayKey := strings.Trim(ss[1], " ")

					left := strings.Trim(ss[0], " ")
					var itemKey string
					var indexKey string
					// (item, index) in list
					if strings.Contains(left, ",") {
						left = strings.Trim(left, "()")
						ss := strings.Split(left, ",")
						itemKey = strings.Trim(ss[0], " ")
						indexKey = strings.Trim(ss[1], " ")
					} else {
						itemKey = left
						indexKey = "$index"
					}

					vFor = &VFor{
						ArrayKey: arrayKey,
						ItemKey:  itemKey,
						IndexKey: indexKey,
					}
				case key == "v-if":
					vIf = &VIf{
						Condition: strings.Trim(attr.Val, " "),
						ElseIf:    nil,
					}
				case nameSpace == "v-slot":
					slotName := key
					propsKey := attr.Val
					// 不应该为空, 否则可能会导致生成的go代码有误
					if propsKey == "" {
						propsKey = "slotProps"
					}
					vSlot = &VSlot{
						SlotName: slotName,
						PropsKey: propsKey,
					}
				case key == "v-else-if":
					vElseIf = &ElseIf{
						Types:     "elseif",
						Condition: strings.Trim(attr.Val, " "),
					}
				case key == "v-else":
					vElse = &ElseIf{
						Types:     "else",
						Condition: strings.Trim(attr.Val, " "),
					}
				case key == "v-html":
					vHtml = strings.Trim(attr.Val, " ")
				case key == "v-text":
					vText = strings.Trim(attr.Val, " ")
				default:
					// 自定义指令
					var name string
					var arg string
					if nameSpace != "-" {
						name = nameSpace
						arg = key
					} else {
						name = key
					}
					ds = append(ds, Directive{
						Name:  name,
						Value: strings.Trim(attr.Val, " "),
						Arg:   arg,
					})
				}
			} else if attr.Key == "class" {
				ss := strings.Split(attr.Val, " ")
				for _, v := range ss {
					if v != "" {
						class = append(class, v)
					}
				}
			} else if attr.Key == "style" {
				ss := strings.Split(attr.Val, ";")
				for _, v := range ss {
					v = strings.Trim(v, " ")
					ss := strings.Split(v, ":")
					if len(ss) != 2 {
						continue
					}
					key := strings.Trim(ss[0], " ")
					val := strings.Trim(ss[1], " ")
					style[key] = val
					styleKeys = append(styleKeys, key)
				}
			} else {
				key := attr.Key
				if attr.Namespace != "" {
					key = attr.Namespace + ":" + attr.Key
				}
				attrs[key] = attr.Val
				attrsKeys = append(attrsKeys, key)
			}
		}

		ch := p.parseList(e.Children)

		v := &VueElement{
			IsRoot:           e.Root,
			TagName:          e.TagName,
			Text:             e.Text,
			Attrs:            attrs,
			AttrsKeys:        attrsKeys,
			Directives:       ds,
			ElseIfConditions: []ElseIf{},
			Class:            class,
			Style:            style,
			StyleKeys:        styleKeys,
			Props:            props,
			Children:         ch,
			VIf:              vIf,
			VFor:             vFor,
			VSlot:            vSlot,
			VElse:            vElse != nil,
			VElseIf:          vElseIf != nil,
			VHtml:            vHtml,
			VText:            vText,
		}

		// 记录vif, 接下来的elseif将与这个节点关联
		if vIf != nil {
			ifVueEle = v
		} else {
			// 如果有vif环境了, 但是中间跳过了, 则需要取消掉vif环境 (v-else 必须与v-if 相邻)
			if vElse == nil && vElseIf == nil {
				ifVueEle = nil
			}
		}

		if vElseIf != nil {
			if ifVueEle == nil {
				panic("v-else-if must below v-if")
			}
			vElseIf.VueElement = v
			ifVueEle.VIf.AddElseIf(vElseIf)
		}
		if vElse != nil {
			if ifVueEle == nil {
				panic("v-else must below v-if")
			}
			vElse.VueElement = v
			ifVueEle.VIf.AddElseIf(vElse)
			ifVueEle = nil
		}

		vs[i] = v
	}

	return vs
}
