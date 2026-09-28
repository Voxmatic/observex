const COLORS: Record<string,[string,string]> = {
  CRITICAL:['rgba(255,77,106,.15)','#ff4d6a'],
  HIGH:['rgba(245,166,35,.15)','#f5a623'],
  MEDIUM:['rgba(168,85,247,.15)','#c084fc'],
  LOW:['rgba(84,106,136,.15)','#546a88'],
  INFO:['rgba(108,114,255,.15)','#8b90ff'],
  healthy:['rgba(15,207,138,.12)','#0fcf8a'],
  warning:['rgba(245,166,35,.12)','#f5a623'],
  critical:['rgba(255,77,106,.12)','#ff4d6a'],
  ok:['rgba(15,207,138,.12)','#0fcf8a'],
  error:['rgba(255,77,106,.12)','#ff4d6a'],
  open:['rgba(255,77,106,.15)','#ff4d6a'],
  resolved:['rgba(15,207,138,.15)','#0fcf8a'],
  investigating:['rgba(245,166,35,.15)','#f5a623'],
  acknowledged:['rgba(108,114,255,.15)','#8b90ff'],
}

export function StatusBadge({ status, size='sm' }: { status:string; size?:'sm'|'xs' }) {
  const [bg,color] = COLORS[status] || COLORS.INFO
  const fontSize = size==='xs'?9:10
  const padding = size==='xs'?'1px 5px':'2px 8px'
  return <span style={{display:'inline-block',background:bg,color,fontSize,fontWeight:700,padding,borderRadius:10,textTransform:'uppercase'}}>{status}</span>
}
