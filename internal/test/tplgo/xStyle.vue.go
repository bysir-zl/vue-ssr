// Code generated by go-vue-ssr: https://github.com/bysir-zl/go-vue-ssr
// src_hash:042df74622f7953e8e5236bd65898968

package tplgo

func (r *Render) Component_xStyle(options *Options) string {
	this := extendMap(r.Prototype, options.Props)
	_ = this
	return r.Tag("div", true, &Options{
		PropsStyle: map[string]interface{}{"color": "#f33"},
		Style:      map[string]string{"font-size": "20px"},
		Slot: map[string]NamedSlotFunc{"default": func(props map[string]interface{}) string {
			return "<span>" + interfaceToStr(lookInterface(this, "text")) + "</span>"
		}},
		P:    options,
		Data: this,
	})
}