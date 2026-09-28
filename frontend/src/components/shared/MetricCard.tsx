import { SparkLine } from './SparkLine'

interface Props {
  label: string
  value: string | number
  unit?: string
  color?: string
  trend?: number[]
  change?: string
  changeColor?: string
  small?: boolean
}

export function MetricCard({ label, value, unit, color='#8fa8cc', trend, change, changeColor, small }: Props) {
  return (
    <div style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:small?8:10,padding:small?'7px 10px':'10px 14px',display:'flex',flexDirection:'column',gap:3}}>
      <div style={{fontSize:small?8:9,color:'#546a88',textTransform:'uppercase',letterSpacing:'.04em'}}>{label}</div>
      <div style={{display:'flex',alignItems:'baseline',gap:4}}>
        <span style={{fontSize:small?16:20,fontWeight:800,color,fontFamily:'Syne,sans-serif'}}>{value}</span>
        {unit&&<span style={{fontSize:small?9:10,color:'#546a88'}}>{unit}</span>}
        {change&&<span style={{fontSize:small?9:10,color:changeColor||'#546a88',fontWeight:600,marginLeft:'auto'}}>{change}</span>}
      </div>
      {trend&&<SparkLine data={trend} width={small?80:120} height={small?18:24} color={color}/>}
    </div>
  )
}
