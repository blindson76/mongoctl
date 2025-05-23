const NOMAD_API = process.argv[2] || 'http://127.0.0.1:4646'
console.log(process.argv[2])
const check = async () => {
    for(;;){
        try{            
            const res = await fetch(`${NOMAD_API}/v1/status/leader`)
            if(res.ok){
                console.log(await res.json())
                return
            }
        }catch(e){
            console.log('fetch error')
            await new Promise(rs=>setTimeout(rs, 2000))
        }
    }
}

check()