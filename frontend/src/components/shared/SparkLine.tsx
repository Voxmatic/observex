interface Props {
  data: number[]
  width?: number
  height?: number
  color?: string
  fill?: boolean
  strokeWidth?: number
}

export function SparkLine({ data, width=120, height=28, color='#6c72ff', fill=true, strokeWidth=1.5 }: Props) {
  if (!data.length) return null
  const max = Math.max(...data, 1)
  const min = Math.min(...data, 0)
  const range = max - min || 1
  const pts = data.map((v,i) => {
    const x = (i/(data.length-1))*width
    const y = height - ((v-min)/range)*(height-4) - 2
    return `${x},${y}`
  })
  const line = pts.join(' ')
  const area = `0,${height} ${line} ${width},${height}`
  return (
    <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} style={{display:'block',overflow:'visible'}}>
      {fill&&<polygon points={area} fill={`${color}18`}/>}
      <polyline points={line} fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinejoin="round" strokeLinecap="round"/>
    </svg>
  )
}
