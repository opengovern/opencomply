import {
    Accordion,
    AccordionBody,
    AccordionHeader,
    Button,
    Card,
    Flex,
    Icon,
    Text,
    Title,
} from '@tremor/react'
import {
    ChevronRightIcon,
    MagnifyingGlassIcon,
} from '@heroicons/react/24/outline'
import Editor from 'react-simple-code-editor'
import 'prismjs/themes/prism.css'
import { highlight, languages } from 'prismjs'
import { useNavigate, useParams } from 'react-router-dom'
import { useAtom } from 'jotai'
import { useEffect, useState } from 'react'
import { useInventoryApiV1QueryList } from '../../../api/inventory.gen'
import { runQueryAtom } from '../../../store'
import { getErrorMessage } from '../../../types/apierror'
import { GithubComKaytuIoKaytuEnginePkgInventoryApiSmartQueryItem } from '../../../api/api'
import { useIntegrationApiV1ConnectorsList } from '../../../api/integration.gen'
import { Box, Cards, Link, SpaceBetween } from '@cloudscape-design/components'

interface IQuery {
    height: any
}


  const GetTierIcon = (tier: string) => {
      if (tier === 'Community') {
          return (
              <svg
                  width="24"
                  height="24"
                  viewBox="0 0 24 24"
                  fill="none"
                  xmlns="http://www.w3.org/2000/svg"
              >
                  <path
                      fill-rule="evenodd"
                      clip-rule="evenodd"
                      d="M11.9034 2.25264C13.1418 2.20921 14.3466 2.70205 15.342 3.69744L20.3003 8.65575C21.2978 9.65321 21.7914 10.8583 21.7473 12.0969C21.7035 13.3273 21.1328 14.5083 20.1572 15.4834L15.4851 20.1555C14.5101 21.1305 13.3293 21.7013 12.0991 21.745C10.8605 21.7891 9.65543 21.2955 8.65754 20.2976L3.69923 15.3393C2.70179 14.3418 2.20839 13.1372 2.25275 11.8988C2.29681 10.6687 2.86785 9.48812 3.84318 8.51277C3.84316 8.51279 3.84319 8.51276 3.84318 8.51277L8.51445 3.84052C9.49052 2.86501 10.6731 2.29579 11.9034 2.25264ZM9.57502 4.90128C9.57495 4.90134 9.57509 4.90121 9.57502 4.90128L4.90395 9.57332C4.1474 10.3299 3.77996 11.1658 3.75179 11.9525C3.7239 12.7308 4.02445 13.5432 4.75989 14.2786L9.7182 19.2369C10.4542 19.9729 11.267 20.2737 12.0457 20.246C12.8327 20.218 13.6685 19.8508 14.4244 19.0949L19.0966 14.4226C19.8529 13.6668 20.2203 12.8307 20.2483 12.0436C20.276 11.2647 19.9751 10.4518 19.2397 9.71641L14.2814 4.7581C13.5487 4.02548 12.7361 3.72436 11.956 3.75172C11.168 3.77935 10.3308 4.146 9.57502 4.90128Z"
                      fill="black"
                  />
                  <path
                      fill-rule="evenodd"
                      clip-rule="evenodd"
                      d="M11.3716 14.3594C10.9772 14.3594 10.6582 14.6784 10.6582 15.0727C10.6582 15.4671 10.9772 15.7861 11.3716 15.7861C11.7659 15.7861 12.0849 15.4671 12.0849 15.0727C12.0849 14.6784 11.7659 14.3594 11.3716 14.3594ZM9.1582 15.0727C9.1582 13.85 10.1488 12.8594 11.3716 12.8594C12.5943 12.8594 13.5849 13.85 13.5849 15.0727C13.5849 16.2955 12.5943 17.2861 11.3716 17.2861C10.1488 17.2861 9.1582 16.2955 9.1582 15.0727Z"
                      fill="black"
                  />
                  <path
                      fill-rule="evenodd"
                      clip-rule="evenodd"
                      d="M15.4751 11.2891C15.0807 11.2891 14.7617 11.6081 14.7617 12.0024C14.7617 12.3968 15.0807 12.7158 15.4751 12.7158C15.8694 12.7158 16.1884 12.3968 16.1884 12.0024C16.1884 11.6081 15.8694 11.2891 15.4751 11.2891ZM13.2617 12.0024C13.2617 10.7797 14.2523 9.78906 15.4751 9.78906C16.6978 9.78906 17.6884 10.7797 17.6884 12.0024C17.6884 13.2252 16.6978 14.2158 15.4751 14.2158C14.2523 14.2158 13.2617 13.2252 13.2617 12.0024Z"
                      fill="black"
                  />
                  <path
                      fill-rule="evenodd"
                      clip-rule="evenodd"
                      d="M11.3716 7.37109C10.9772 7.37109 10.6582 7.69012 10.6582 8.08446C10.6582 8.47879 10.9772 8.79782 11.3716 8.79782C11.7659 8.79782 12.0849 8.47879 12.0849 8.08446C12.0849 7.69012 11.7659 7.37109 11.3716 7.37109ZM9.1582 8.08446C9.1582 6.8617 10.1488 5.87109 11.3716 5.87109C12.5943 5.87109 13.5849 6.8617 13.5849 8.08446C13.5849 9.30722 12.5943 10.2978 11.3716 10.2978C10.1488 10.2978 9.1582 9.30722 9.1582 8.08446Z"
                      fill="black"
                  />
                  <path
                      fill-rule="evenodd"
                      clip-rule="evenodd"
                      d="M7.98456 4.53877C8.27813 4.24655 8.753 4.24764 9.04522 4.5412L10.9464 6.45116C11.2386 6.74473 11.2376 7.2196 10.944 7.51182C10.6504 7.80404 10.1755 7.80295 9.88333 7.50938L7.98212 5.59942C7.68991 5.30586 7.691 4.83098 7.98456 4.53877Z"
                      fill="black"
                  />
                  <path
                      fill-rule="evenodd"
                      clip-rule="evenodd"
                      d="M11.9501 8.51264C12.243 8.21975 12.7179 8.21975 13.0108 8.51264L14.9558 10.4576C15.2487 10.7505 15.2487 11.2254 14.9558 11.5183C14.6629 11.8112 14.188 11.8112 13.8951 11.5183L11.9501 9.5733C11.6572 9.28041 11.6572 8.80553 11.9501 8.51264Z"
                      fill="black"
                  />
                  <path
                      fill-rule="evenodd"
                      clip-rule="evenodd"
                      d="M11.373 8.79688C11.7873 8.79688 12.123 9.13266 12.123 9.54688V13.6096C12.123 14.0239 11.7873 14.3596 11.373 14.3596C10.9588 14.3596 10.623 14.0239 10.623 13.6096V9.54688C10.623 9.13266 10.9588 8.79688 11.373 8.79688Z"
                      fill="black"
                  />
              </svg>
          )
      } else if (tier === 'Premium') {
          return (
              <svg
                  width="24"
                  height="24"
                  viewBox="0 0 24 24"
                  fill="none"
                  xmlns="http://www.w3.org/2000/svg"
              >
                  <path
                      d="M3.51047 11.5274L10.8163 19.5542C11.4507 20.2518 12.5482 20.2518 13.1825 19.5542L20.4894 11.5264C21.1004 10.8561 21.1704 9.85395 20.6596 9.10381L17.7174 4.78197C17.3526 4.24685 16.7455 3.92578 16.0975 3.92578H7.91111C7.26312 3.92578 6.65698 4.24588 6.29212 4.781L3.34117 9.10381C2.8294 9.85298 2.89946 10.8561 3.51047 11.5274Z"
                      stroke="black"
                      stroke-width="1.5"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                  />
                  <path
                      d="M13.9989 3.92578L15.3659 9.10381C15.5643 9.85395 15.5371 10.8561 15.2997 11.5264L12.4597 19.5542C12.2135 20.2518 11.7864 20.2518 11.5402 19.5542L8.7002 11.5274C8.4628 10.8561 8.43556 9.85298 8.63501 9.10381L10.0059 3.92578"
                      stroke="black"
                      stroke-width="1.5"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                  />
                  <path
                      d="M3.00781 10.2031H20.9908"
                      stroke="black"
                      stroke-width="1.5"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                  />
              </svg>
          )
      } else if (tier === 'Free') {
          return (
              <svg
                  width="24px"
                  height="24px"
                  viewBox="0 0 24 24"
                  version="1.1"
                  xmlns="http://www.w3.org/2000/svg"
              >
                  <title>Iconly/Two-tone/Unlock</title>
                  <g
                      id="Iconly/Two-tone/Unlock"
                      stroke="none"
                      stroke-width="1"
                      fill="none"
                      fill-rule="evenodd"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                  >
                      <g
                          id="Unlock"
                          transform="translate(4.500000, 2.500000)"
                          stroke="#000000"
                          stroke-width="1.5"
                      >
                          <path
                              d="M11.9242,3.06203682 C11.3072,1.28003682 9.6142,0 7.6222,0 C5.1092,-0.00996317625 3.0632,2.01803682 3.0522,4.53103682 L3.0522,4.55103682 L3.0522,6.69803682"
                              id="Stroke-1"
                              opacity="0.400000006"
                          ></path>
                          <path
                              d="M11.433,18.5004368 L3.792,18.5004368 C1.698,18.5004368 1.13686838e-13,16.8024368 1.13686838e-13,14.7074368 L1.13686838e-13,10.4194368 C1.13686838e-13,8.32443682 1.698,6.62643682 3.792,6.62643682 L11.433,6.62643682 C13.527,6.62643682 15.225,8.32443682 15.225,10.4194368 L15.225,14.7074368 C15.225,16.8024368 13.527,18.5004368 11.433,18.5004368 Z"
                              id="Stroke-3"
                          ></path>
                          <line
                              x1="7.6127"
                              y1="11.4526368"
                              x2="7.6127"
                              y2="13.6746368"
                              id="Stroke-5"
                          ></line>
                      </g>
                  </g>
              </svg>
          )
      } else {
          return (
              <svg
                  width="24"
                  height="24"
                  viewBox="0 0 24 24"
                  fill="none"
                  xmlns="http://www.w3.org/2000/svg"
              >
                  <path
                      d="M3.51047 11.5274L10.8163 19.5542C11.4507 20.2518 12.5482 20.2518 13.1825 19.5542L20.4894 11.5264C21.1004 10.8561 21.1704 9.85395 20.6596 9.10381L17.7174 4.78197C17.3526 4.24685 16.7455 3.92578 16.0975 3.92578H7.91111C7.26312 3.92578 6.65698 4.24588 6.29212 4.781L3.34117 9.10381C2.8294 9.85298 2.89946 10.8561 3.51047 11.5274Z"
                      stroke="black"
                      stroke-width="1.5"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                  />
                  <path
                      d="M13.9989 3.92578L15.3659 9.10381C15.5643 9.85395 15.5371 10.8561 15.2997 11.5264L12.4597 19.5542C12.2135 20.2518 11.7864 20.2518 11.5402 19.5542L8.7002 11.5274C8.4628 10.8561 8.43556 9.85298 8.63501 9.10381L10.0059 3.92578"
                      stroke="black"
                      stroke-width="1.5"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                  />
                  <path
                      d="M3.00781 10.2031H20.9908"
                      stroke="black"
                      stroke-width="1.5"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                  />
              </svg>
          )
      }
  }  
export default function Integrations({ height }: IQuery) {
    const workspace = useParams<{ ws: string }>().ws
    const navigate = useNavigate()
    const [runQuery, setRunQuery] = useAtom(runQueryAtom)
    const [open, setOpen] = useState(0)
const {
    response: responseConnectors,
    isLoading: connectorsLoading,
    sendNow: getList,
    
} = useIntegrationApiV1ConnectorsList(9, 1,undefined,'count','desc',{},false)

useEffect(()=>{getList(4,1,'count','desc',false)},[])
    
    return (
        <Card
            className="h-full w-3/4 overflow-scroll no-scrollbar"
            style={{ maxHeight: `${height}px` }}
        >
            <Flex justifyContent="between" alignItems='center'>
                <Flex justifyContent="start" className="gap-2 ">
                    <Icon icon={MagnifyingGlassIcon} className="p-0" />
                    <Title className="font-semibold">Integrations</Title>
                </Flex>
                <a
                    target="__blank"
                    href={`/integrations`}
                    className=" cursor-pointer"
                >
                    <Button
                        size="xs"
                        variant="light"
                        icon={ChevronRightIcon}
                        iconPosition="right"
                        className="my-3"
                        // onClick={() => {
                        //     navigate(`/cloudql?tab_id=0`)
                        // }}
                    >
                        see all 
                    </Button>
                </a>
            </Flex>
            {/* {isLoading
                ? [1, 2, 3, 4, 5].map((i) => (
                      <Accordion
                          className={`w-full border-0 ${
                              i < 4 ? 'border-b border-b-gray-200' : ''
                          } !rounded-none bg-transparent ${
                              isLoading ? 'animate-pulse' : ''
                          }`}
                      >
                          <AccordionHeader className="pl-0 pr-0.5 py-4 bg-transparent flex justify-start">
                              <div className="h-5 w-32 bg-slate-200 dark:bg-slate-700 rounded" />
                          </AccordionHeader>
                      </Accordion>
                  ))
                : getQueries(queries)
                      ?.sort((a, b) => {
                          // eslint-disable-next-line @typescript-eslint/ban-ts-comment
                          // @ts-ignore
                          if (a.title < b.title) {
                              return -1
                          }
                          // eslint-disable-next-line @typescript-eslint/ban-ts-comment
                          // @ts-ignore
                          if (a.title > b.title) {
                              return 1
                          }
                          return 0
                      })
                      .map((q, i) => (
                          <Accordion
                              // eslint-disable-next-line react/no-array-index-key
                              key={`query-${i}-${open}`}
                              className={`w-full border-0 ${
                                  i < 4 ? 'border-b border-b-gray-200' : ''
                              } !rounded-none bg-transparent`}
                              defaultOpen={i === open}
                              onClick={() => {
                                  if (i !== open) {
                                      setOpen(i)
                                  }
                              }}
                          >
                              <AccordionHeader className="pl-0 pr-0.5 py-4 bg-transparent flex justify-start">
                                  <Text className="text-gray-800 !text-base line-clamp-1">
                                      {q?.title}
                                  </Text>
                              </AccordionHeader>
                              <AccordionBody className="p-0 w-full pr-0.5 cursor-default ">
                                  <Editor
                                      onValueChange={(text) => {
                                          console.log('')
                                      }}
                                      highlight={(text) =>
                                          highlight(text, languages.sql, 'sql')
                                      }
                                      value={q?.query || ''}
                                      className="w-full bg-gray-100 rounded p-5 dark:bg-gray-800 font-mono text-sm h-full no-scrollbar"
                                      style={{
                                          color: 'white !important',
                                          minHeight: '60px',
                                          overflowY: 'scroll',
                                          padding: '2rem!important',
                                      }}
                                      placeholder="-- write your SQL query here"
                                  />
                                  <Button
                                      size="xs"
                                      variant="light"
                                      icon={ChevronRightIcon}
                                      iconPosition="right"
                                      className="my-3"
                                      onClick={() => {
                                          setRunQuery(q?.query || '')
                                          navigate(
                                              `/cloudql?tab_id=1`
                                          )
                                      }}
                                  >
                                      Run Query
                                  </Button>
                              </AccordionBody>
                          </Accordion>
                      ))}
            {error && (
                <Flex
                    flexDirection="col"
                    justifyContent="between"
                    className="absolute top-0 w-full left-0 h-full backdrop-blur"
                >
                    <Flex
                        flexDirection="col"
                        justifyContent="center"
                        alignItems="center"
                    >
                        <Title className="mt-6">Failed to load component</Title>
                        <Text className="mt-2">{getErrorMessage(error)}</Text>
                    </Flex>
                    <Button
                        variant="secondary"
                        className="mb-6"
                        color="slate"
                        onClick={refresh}
                    >
                        Try Again
                    </Button>
                </Flex>
            )} */}
            <Cards
                ariaLabels={{
                    itemSelectionLabel: (e, t) => `select ${t.name}`,
                    selectionGroupLabel: 'Item selection',
                }}
                onSelectionChange={({ detail }) => {
                    // const item = detail?.selectedItems[0]
                    // if (item.tier === 'Community' && item?.SourceCode != '') {
                    //     navigate('/integrations/' + item.schema_id + '/schema')
                    // } else {
                    //     setOpen(true)
                    // }
                    // setSelectedItems(detail?.selectedItems ?? []);
                }}
                selectedItems={[]}
                cardDefinition={{
                    header: (item) => (
                        <Link
                            className="w-100"
                            onClick={() => {
                                // if (item.tier === 'Community') {
                                //     navigate(
                                //         '/integrations/' +
                                //             item.schema_id +
                                //             '/schema'
                                //     )
                                // } else {
                                //     // setOpen(true);
                                // }
                            }}
                        >
                            <div className="w-100 flex flex-row justify-between">
                                <span>{item.name}</span>
                                {/* <div className="flex flex-row gap-1 items-center">
                                    {GetTierIcon(item.tier)}
                                    <span className="text-white">{item.tier}</span>
                                </div> */}
                            </div>
                        </Link>
                    ),
                    sections: [
                        {
                            id: 'logo',
                            // header :(<>
                            //     <div className="flex justify-end">
                            //         <span>{'Status'}</span>
                            //     </div>
                            // </>),

                            content: (item) => (
                                <div className="w-100 flex flex-row items-center  justify-between  ">
                                    <img
                                        className="w-[50px] h-[50px]"
                                        src={item.logo}
                                        onError={(e) => {
                                            e.currentTarget.onerror = null
                                            e.currentTarget.src =
                                                'https://raw.githubusercontent.com/opengovern/website/main/connectors/icons/default.svg'
                                        }}
                                        alt="placeholder"
                                    />
                                    {/* <span>{item.status ? 'Enabled' : 'Disable'}</span> */}
                                </div>
                            ),
                        },
                        // {
                        //     id: 'description',
                        //     header: (
                        //         <>
                        //             <div className="flex justify-between">
                        //                 <span>{'Description'}</span>
                        //                 <span>{'Table'}</span>
                        //             </div>
                        //         </>
                        //     ),
                        //     content: (item) => (
                        //         <>
                        //             <div className="flex justify-between">
                        //                 <span className="max-w-60">
                        //                     {item.description}
                        //                 </span>
                        //                 <span>
                        //                     {item.count ? item.count : '--'}
                        //                 </span>
                        //             </div>
                        //         </>
                        //     ),
                        // },
                        // {
                        //     id: 'status',
                        //     header: 'Status',
                        //     content: (item) =>
                        //         item.status ? 'Enabled' : 'Disabled',
                        //     width: 70,
                        // },
                        {
                            id: 'integrattoin',
                            header: 'Integrations',
                            content: (item) =>
                              item?.count ? item.count : '--',
                            width:50,
                        },
                        // {
                        //   id: "tier",
                        //   header: "Tier",
                        //   content: (item) => item.tier,
                        //   width: 85,
                        // },
                        // {
                        //   id: "tables",
                        //   header: "Table",
                        //   content: (item) => (item.count ? item.count : "--"),
                        //   width: 15,
                        // },
                    ],
                }}
                cardsPerRow={[{ cards: 1 }]}
                // @ts-ignore
                items={responseConnectors?.integration_types?.map((type) => {
                    return {
                        id: type.id,
                        tier: type.tier,
                        status: type.enabled,
                        // description: type.Description,
                        name: type.title,
                        count: type?.count?.total,
                        // schema_id: type?.schema_ids[0],
                        // SourceCode: type.SourceCode,
                        logo: `https://raw.githubusercontent.com/opengovern/website/main/connectors/icons/${type.logo}`,
                    }
                })}
                loadingText="Loading resources"
                stickyHeader
                entireCardClickable
                variant="full-page"
                selectionType="single"
                trackBy="name"
                empty={
                    <Box
                        margin={{ vertical: 'xs' }}
                        textAlign="center"
                        color="inherit"
                    >
                        <SpaceBetween size="m">
                            <b>No resources</b>
                        </SpaceBetween>
                    </Box>
                }
            />
        </Card>
    )
}
