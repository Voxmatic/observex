import { useState } from 'react'
import { Clock, ChevronDown } from 'lucide-react'

const PRESETS = [
  { label:'5m',  value:'5m',  ms:300000 },
  { label:'15m', value:'15m', ms:900000 },
  { label:'30m', value:'30m', ms:1800000 },
  { label:'1h',  value:'1h',  ms:3600000 },
  { label:'3h',  value:'3h',  ms:10800000 },
  { label:'6h',  value:'6h',  ms:21600000 },
  { label:'12h', value:'12h', ms:43200000 },
  { label:'24h', value:'24h', ms:86400000 },
  { label:'3d',  value:'3d',  ms:259200000 },
  { label:'7d',  value:'7d',  ms:604800000 },
  { label:'30d', value:'30d', ms:2592000000 },
]

interface Props {
  value: string
  onChange: (v:string) => void
  compact?: boolean
}

export function TimeRangePicker({ value, onChange, compact }: Props) {
  const [open, setOpen] = useState(false)
  return (
    <div style={{position:'relative',display:'inline-flex'}}>
      <button onClick={()=>setOpen(!open)}
        style={{display:'flex',alignItems:'center',gap:5,background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:8,padding:compact?'4px 8px':'6px 12px',color:'#c8d8ef',fontSize:compact?10:12,cursor:'pointer',fontWeight:600}}>
        <Clock size={compact?10:12} style={{color:'#6c72ff'}}/>
        {value}
        <ChevronDown size={compact?8:10} style={{color:'#546a88'}}/>
      </button>
      {open&&(
        <div style={{position:'absolute',top:'100%',right:0,marginTop:4,background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:4,zIndex:50,display:'flex',flexWrap:'wrap',gap:2,width:compact?180:220,boxShadow:'0 8px 24px rgba(0,0,0,.5)'}}>
          {PRESETS.map(p=>(
            <button key={p.value} onClick={()=>{onChange(p.value);setOpen(false)}}
              style={{padding:compact?'4px 8px':'5px 10px',borderRadius:6,background:value===p.value?'rgba(108,114,255,.2)':'transparent',border:`1px solid ${value===p.value?'#6c72ff':'transparent'}`,color:value===p.value?'#8b90ff':'#546a88',fontSize:compact?10:11,cursor:'pointer',fontWeight:600}}>
              {p.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
