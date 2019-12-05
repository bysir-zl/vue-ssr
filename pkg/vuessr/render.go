package vuessr

import (
	"fmt"
	"github.com/bysir-zl/go-vue-ssr/internal/pkg/log"
	"github.com/bysir-zl/go-vue-ssr/pkg/vuessr/ast_from_api"
	"regexp"
	"strings"
)

type App struct {
	Components map[string]struct{} // name=>node
}

// 用来生成Option代码所需要的数据
type OptionsGen struct {
	Props           map[string]string // 上级传递的 数据(包含了class和style)
	Attrs           map[string]string // 上级传递的 静态的attrs (除去class和style), 只会作用在root节点
	Class           []string          // 静态class
	Style           map[string]string // 静态style
	StyleKeys       []string          // 样式的key, 用来保证顺序
	Slot            map[string]string // 插槽节点
	DefaultSlotCode string            // 子节点code, 用于默认的插槽
	NamedSlotCode   map[string]string // 具名插槽
	Directives      []Directive       // 指令代码
}

func sliceStringToGoCode(m []string) string {
	if len(m) == 0 {
		return "nil"
	}
	c := strings.Join(m, `","`)
	c = fmt.Sprintf(`[]string{"%s"}`, c)
	return c
}

func mapStringToGoCode(m map[string]string) string {
	if len(m) == 0 {
		return "nil"
	}
	c := "map[string]string"
	c += "{"
	for k, v := range m {
		c += fmt.Sprintf(`"%s": "%s",`, k, v)
	}
	c += "}"

	return c
}

func mapGoCodeToCode(m map[string]string, valueType string) string {
	c := "map[string]" + valueType
	c += "{"
	for k, v := range m {
		c += fmt.Sprintf(`"%s": %s,`, k, v)
	}
	c += "}"

	return c
}

func sliceToGoCode(m []string) string {
	c := "[]string"
	c += "{"
	for _, v := range m {
		c += fmt.Sprintf(`"%s", `, v)
	}
	c += "}"

	return c
}

// 根据js代码生成go代码(基于js AST)
func mapJsCodeToCode(m map[string]string) string {
	if len(m) == 0 {
		return "nil"
	}
	props := "map[string]interface{}"
	props += "{"
	for k, v := range m {
		valueCode, err := ast_from_api.Js2Go(v, DataKey)
		if err != nil {
			log.Panicf("%v, %s", err, v)
		}
		props += fmt.Sprintf(`"%s": %s,`, k, valueCode)
	}
	props += "}"

	return props
}

func (o *OptionsGen) ToGoCode() string {
	c := "&Options{"

	if len(o.Props) != 0 {
		// class
		cCode := getPropsClass(o.Props)
		if cCode != "nil" {
			delete(o.Props, "class")
			c += fmt.Sprintf("PropsClass: %s, \n", cCode)
		}
		// style
		cStyle := getPropsStyle(o.Props)
		if cStyle != "nil" {
			delete(o.Props, "style")
			c += fmt.Sprintf("PropsStyle: %s, \n", cStyle)
		}

		// 除了class/style的props
		if len(o.Props) != 0 {
			c += fmt.Sprintf("Props: %s, \n", mapJsCodeToCode(o.Props))
		}
	}

	if len(o.Attrs) != 0 {
		c += fmt.Sprintf("Attrs: %s,\n", mapStringToGoCode(o.Attrs))
	}
	if len(o.Class) != 0 {
		c += fmt.Sprintf("Class: %s,\n", sliceToGoCode(o.Class))
	}
	if len(o.Style) != 0 {
		c += fmt.Sprintf("Style: %s,\n", mapStringToGoCode(o.Style))
	}
	if len(o.StyleKeys) != 0 {
		c += fmt.Sprintf("StyleKeys: %s,\n", sliceToGoCode(o.StyleKeys))
	}

	// slot
	slot := map[string]string{}

	children := o.DefaultSlotCode
	if children == "" {
		children = `""`
	}
	slot["default"] = fmt.Sprintf(`func (props map[string]interface{})string{return %s}`, children)

	for k, v := range o.NamedSlotCode {
		slot[k] = v
	}
	c += fmt.Sprintf("Slot: %s,\n", mapGoCodeToCode(slot, "namedSlotFunc"))

	// p
	c += fmt.Sprintf("P: options,\n")

	// directive
	if len(o.Directives) != 0 {
		// 数组
		dir := "[]directive{\n"
		for _, v := range o.Directives {
			valueCode := "nil"
			if v.Value != "" {
				var err error
				valueCode, err = ast_from_api.Js2Go(v.Value, DataKey)
				if err != nil {
					panic(err)
				}
			}
			dir += fmt.Sprintf("{Name: \"%s\", Value: %s},\n", v.Name, valueCode)
		}
		dir += "}"

		c += fmt.Sprintf("Directives: %s,\n", dir)
	}

	c += "}"
	return c
}

// 生成代码中的key
const (
	DataKey = "this" // 变量作用域的key, 相当于js的this.
	SlotKey = "options.Slot"
)

// 组件渲染,
// 如果该组件被components注册, 则使用Element渲染.
//
// 用节点直接渲染可能会有的性能问题:
// - 处理文字时会使用正则来匹配{{变量, 会消耗过多性能
// - 很多没有变量的节点可以被预先处理成字符串, 就不会走递归流程
//

// 每个组件都是一个func或者是一个字符串
// slot: 子级代码
func (e *VueElement) GenCode(app *App) (code string, namedSlotCode map[string]string) {
	var eleCode = ""

	defaultSlotCode := ""

	namedSlotCode = map[string]string{}
	if len(e.Children) != 0 {
		for _, v := range e.Children {
			// 跳过生成else节点的代码, 真正生成else节点的代码在if节点中
			if v.VElse || v.VElseIf {
				continue
			}
			childCode, childNamedSlotCode := v.GenCode(app)
			if defaultSlotCode != "" {
				defaultSlotCode += "+"
			}
			defaultSlotCode += childCode

			for k, v := range childNamedSlotCode {
				namedSlotCode[k] = v
			}
		}
	}

	if defaultSlotCode == "" {
		defaultSlotCode = `""`
	}

	// 调用组件
	_, exist := app.Components[e.TagName]
	if exist {
		options := OptionsGen{
			StyleKeys:       e.StyleKeys,
			Class:           e.Class,
			Attrs:           e.Attrs,
			Props:           e.Props,
			Style:           e.Style,
			DefaultSlotCode: defaultSlotCode,
			NamedSlotCode:   namedSlotCode,
			Directives:      e.Directives,
		}
		optionsCode := options.ToGoCode()
		eleCode = fmt.Sprintf("r.Component_%s(%s)", e.TagName, optionsCode)
	} else if e.TagName == "template" {
		// 使用子级
		eleCode = defaultSlotCode
	} else if e.TagName == "__string" {
		// 纯字符串节点
		text := strings.Replace(e.Text, "\n", `\n`, -1)
		// 处理变量
		text = injectVal(text)
		eleCode = fmt.Sprintf(`"%s"`, text)
	} else {
		// 基础html标签

		// 判断节点是否是动态节点, 动态则使用r.Tag渲染节点, 否则使用字符串拼接
		// 动态节点
		// - 自定义指令: 在指令中会修改任何一个属性(class/style/attr...), 所以是动态的
		// - 组件的root节点: root节点会继承上层传递的(class/style/attr)

		// 动态节点
		if e.IsRoot || len(e.Directives) != 0 {
			options := OptionsGen{
				Props:           e.Props,
				Attrs:           e.Attrs,
				Class:           e.Class,
				Style:           e.Style,
				StyleKeys:       e.StyleKeys,
				Slot:            nil,
				DefaultSlotCode: defaultSlotCode,
				NamedSlotCode:   namedSlotCode,
				Directives:      e.Directives,
			}

			optionsCode := options.ToGoCode()

			eleCode = fmt.Sprintf(`r.Tag("%s", %v, %s)`, e.TagName, e.IsRoot, optionsCode)
		} else {
			// 静态节点
			attrs := genAttrCode(e)
			// 内联元素, slot应该放在标签里
			eleCode = fmt.Sprintf(`"<%s"+%s+">"+%s+"</%s>"`, e.TagName, attrs, defaultSlotCode, e.TagName)
		}
	}

	// 优先级 vSlot > vIf > vFor, 所以先处理VFor

	if e.VFor != nil {
		eleCode = genVFor(e.VFor, eleCode)
	}

	if e.VSlot != nil {
		var namedSlotCode2 map[string]string
		eleCode, namedSlotCode2 = genVSlot(e.VSlot, eleCode)
		for i, v := range namedSlotCode2 {
			namedSlotCode[i] = v
		}
	}

	if e.VIf != nil {
		var namedSlotCode2 map[string]string
		eleCode, namedSlotCode2 = genVIf(e.VIf, eleCode, app)
		for i, v := range namedSlotCode2 {
			namedSlotCode[i] = v
		}
	}

	return eleCode, namedSlotCode
}

func genVIf(e *VIf, srcCode string, app *App) (code string, namedSlotCode map[string]string) {
	// 自己的conditions
	condition, err := ast_from_api.Js2Go(e.Condition, DataKey)
	if err != nil {
		panic(err)
	}
	namedSlotCode = map[string]string{}

	// open if and func
	code = fmt.Sprintf(`func ()string{
if interfaceToBool(%s) {return %s`, condition, srcCode)
	// 继续处理else节点
	for _, v := range e.ElseIf {
		eleCode, namedSlotCode2 := v.VueElement.GenCode(app)
		for k, v := range namedSlotCode2 {
			namedSlotCode[k] = v
		}
		switch v.Types {
		case "else":
			code += fmt.Sprintf(`} else { return %s`, eleCode)
		case "elseif":
			condition, err := ast_from_api.Js2Go(v.Condition, DataKey)
			if err != nil {
				panic(err)
			}
			code += fmt.Sprintf(`} else if interfaceToBool(%s) { return %s`, condition, eleCode)
		}
	}

	// close if and func
	code += `
}
return ""
}()`
	return
}

func genVSlot(e *VSlot, srcCode string) (code string, namedSlotCode map[string]string) {
	namedSlotCode = map[string]string{
		e.SlotName: fmt.Sprintf(`func(props map[string]interface{}) string{
	%s := extendMap(map[string]interface{}{"%s": props}, %s)
_ = %s
return %s
}`, DataKey, e.PropsKey, DataKey, DataKey, srcCode),
	}

	// 插槽会将原来的子代码去掉, 并将代码放在namedSlot里.
	code = `""`
	return
}
func genVFor(e *VFor, srcCode string) (code string) {
	vfArray := e.ArrayKey
	vfItem := e.ItemKey
	vfIndex := e.IndexKey
	// 将自己for, 将子代码的data字段覆盖, 实现作用域的修改
	return fmt.Sprintf(`func ()string{
  var c = ""

  for index, item := range lookInterfaceToSlice(%s, "%s") {
    c += func(xdata map[string]interface{}) string{
        %s := extendMap(map[string]interface{}{
          "%s": index,
          "%s": item,
        }, xdata)

        return %s
    }(%s)
  }
return c
}()`, DataKey, vfArray, DataKey, vfIndex, vfItem, srcCode, DataKey)
}

func NewApp() *App {
	return &App{
		Components: map[string]struct{}{
			"component": {},
			"slot":      {},
		},
	}
}

func (a *App) Component(name string) {
	a.Components[name] = struct {
	}{}
}

// 处理 {{}} 变量
func injectVal(src string) (to string) {
	reg := regexp.MustCompile(`\{\{.+?\}\}`)

	src = reg.ReplaceAllStringFunc(src, func(s string) string {
		key := s[2 : len(s)-2]

		goCode, err := ast_from_api.Js2Go(key, DataKey)
		if err != nil {
			panic(err)
		}
		return fmt.Sprintf(`"+interfaceToStr(%s)+"`, goCode)
	})

	return src
}
